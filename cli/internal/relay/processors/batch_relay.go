// SPDX-License-Identifier: Apache-2.0

package processors

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	"github.com/cosmos/ibc/cli/internal/chains"
	"github.com/cosmos/ibc/cli/internal/relay/prover"
	"github.com/cosmos/ibc/cli/internal/relay/txbuilder"
	"github.com/cosmos/ibc/cli/internal/txsubmitter"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

// findPacketEvent returns the event among events matching sequence and clientID
func findPacketEvent(
	events []v2.PacketEvent,
	sequence uint64,
	clientID string,
) (v2.PacketEvent, error) {
	for _, event := range events {
		if event.Packet.Sequence != sequence || event.Packet.SourceClient != clientID {
			continue
		}

		return event, nil
	}

	return v2.PacketEvent{}, errors.Errorf("no packet event for sequence %d client %q", sequence, clientID)
}

func findPacketEventAtOrBeforeHeight(
	events []v2.PacketEvent,
	sequence uint64,
	clientID string,
	maxHeight uint64,
) (v2.PacketEvent, error) {
	event, err := findPacketEvent(events, sequence, clientID)
	if err != nil {
		return v2.PacketEvent{}, err
	}
	if event.Height > maxHeight {
		return v2.PacketEvent{}, errors.Errorf(
			"packet observed at height %d exceeds maximum height %d", event.Height, maxHeight,
		)
	}
	return event, nil
}

// proofKindFor maps relayKind to the proof claim it requires
func proofKindFor(relayKind v2.RelayKind) v2.ProofKind {
	switch relayKind {
	case v2.RelayKindRecv:
		return v2.ProofKindPacketCommitment
	case v2.RelayKindAck:
		return v2.ProofKindAcknowledgement
	case v2.RelayKindTimeout:
		return v2.ProofKindReceiptAbsence
	default:
		return v2.ProofKindUnknown
	}
}

// relayPackets generates client update payloads and per-packet proofs for
// events at proofHeight, asks txBuilder for the resulting transactions, and
// submits them via txSubmitter.
func relayPackets(
	ctx context.Context,
	logger *slog.Logger,
	chainClient chains.Client,
	prover prover.Prover,
	txBuilder txbuilder.TxBuilder,
	txSubmitter txsubmitter.TxSubmitter,
	clientID string,
	relayKind v2.RelayKind,
	proofHeight uint64,
	events []v2.PacketEvent,
) (*v2.Submission, error) {
	sequences := make([]uint64, len(events))
	for i, event := range events {
		sequences[i] = event.Packet.Sequence
	}

	logger = logger.With("kind", relayKind, "clientID", clientID, "proofHeight", proofHeight, "sequences", sequences)
	logger.Debug("Relaying packets")

	clientUpdatePayloads, err := prover.ClientUpdatePayloads(ctx, proofHeight)
	if err != nil {
		return nil, errors.Wrap(err, "generating client update payloads")
	}

	packets := make([]channeltypesv2.Packet, len(events))
	var acknowledgements []channeltypesv2.Acknowledgement
	if relayKind == v2.RelayKindAck {
		acknowledgements = make([]channeltypesv2.Acknowledgement, len(events))
	}
	for i, event := range events {
		packets[i] = event.Packet
		if relayKind == v2.RelayKindAck {
			acknowledgements[i] = channeltypesv2.NewAcknowledgement(event.Acks...)
		}
	}

	packetProofs, err := prover.PacketProofs(ctx, proofHeight, proofKindFor(relayKind), packets, acknowledgements)
	if err != nil {
		return nil, errors.Wrap(err, "generating packet proofs")
	}

	items := make([]v2.PacketRelayItem, len(events))
	for i, event := range events {
		items[i] = v2.PacketRelayItem{
			Kind:        relayKind,
			Packet:      event.Packet,
			Acks:        event.Acks,
			Proof:       packetProofs[i],
			ProofHeight: proofHeight,
		}
	}

	waitCtx, cancel := context.WithTimeout(ctx, waitForChainTimeout)
	defer cancel()

	if err = chainClient.WaitForChain(waitCtx); err != nil {
		return nil, errors.Wrap(err, "waiting for chain")
	}

	relay := clientRelay{
		logger:      logger,
		txBuilder:   txBuilder,
		txSubmitter: txSubmitter,
		clientID:    clientID,
	}

	// More than one update steps the client across validator set changes.
	// All but the last go in transactions of their own, so however long the
	// chain, no transaction outgrows the size and gas limits a node accepts.
	// Each is mined before the next is submitted: gas is estimated against the
	// latest block, and each update verifies from the state the one before
	// leaves.
	last := max(len(clientUpdatePayloads)-1, 0)
	for i, payload := range clientUpdatePayloads[:last] {
		if err = relay.sendUpdate(ctx, payload, i+1, len(clientUpdatePayloads)); err != nil {
			return nil, errors.Wrapf(err, "client update %d of %d", i+1, len(clientUpdatePayloads))
		}
	}

	submission, err := relay.send(ctx, clientUpdatePayloads[last:], items)
	if err != nil {
		return nil, err
	}
	chainID := chainClient.ChainID()
	logger.Info("Relay transaction submitted", "chainID", chainID, "txHash", submission.TxHash)

	metrics.txSubmitted(ctx, chainID, clientID, submission.TxHash)

	return submission, nil
}

const (
	// updateTxPollInterval is how often an update-only transaction is checked
	// for inclusion before the next one is built.
	updateTxPollInterval = time.Second
	// updateTxLookupTimeout is how long receipt lookups for an update-only
	// transaction may keep failing before the attempt gives up. How long one
	// may stay unmined is up to the tx submitter's pending timeout.
	updateTxLookupTimeout = 10 * time.Minute
)

// clientRelay builds, simulates and submits relay transactions for one client.
type clientRelay struct {
	logger      *slog.Logger
	txBuilder   txbuilder.TxBuilder
	txSubmitter txsubmitter.TxSubmitter
	clientID    string
}

// send builds, simulates and submits one relay transaction.
func (r clientRelay) send(ctx context.Context, payloads [][]byte, items []v2.PacketRelayItem) (*v2.Submission, error) {
	relayTxs, err := r.txBuilder.BuildRelayTxs(v2.ClientUpdate{ClientID: r.clientID, Payloads: payloads}, items)
	if err != nil {
		return nil, errors.Wrap(err, "building relay tx")
	}
	if len(relayTxs) != 1 {
		return nil, errors.Errorf("expected exactly one relay tx, got %d", len(relayTxs))
	}

	relayTx := relayTxs[0]
	simulation, err := r.txSubmitter.SimulateRelayTx(ctx, relayTx)
	if err != nil {
		return nil, errors.Wrap(err, "simulating relay tx")
	}
	if err = validateRelaySimulation(ctx, r.logger, simulation); err != nil {
		return nil, err
	}

	submission, err := r.txSubmitter.Submit(ctx, v2.TxIntent{
		To:   common.BytesToAddress(relayTx.To).Hex(),
		Data: relayTx.Data,
	})
	if err != nil {
		return nil, errors.Wrap(err, "submitting relay tx")
	}

	return submission, nil
}

// sendUpdate submits payload, update n of total, in a transaction of its own
// and waits for it to be mined. An update the client already holds is sent
// all the same: it may be a stalled transaction still pending, which only a
// resend replaces with one that carries the update.
func (r clientRelay) sendUpdate(ctx context.Context, payload []byte, n, total int) error {
	submission, err := r.send(ctx, [][]byte{payload}, nil)
	if err != nil {
		return err
	}
	r.logger.Info("Submitted client update tx", "update", n, "of", total, "txHash", submission.TxHash)

	return r.waitForInclusion(ctx, submission)
}

// waitForInclusion polls until submission is mined successfully, failing if
// it reverted, stayed pending past the tx submitter's retry expiry, or could
// not be looked up for updateTxLookupTimeout. The next attempt then resends
// the update.
func (r clientRelay) waitForInclusion(ctx context.Context, submission *v2.Submission) error {
	ticker := time.NewTicker(updateTxPollInterval)
	defer ticker.Stop()

	var failingSince time.Time
	for {
		retry, err := r.txSubmitter.ShouldRetry(ctx, submission.TxHash, submission.SubmittedAt)
		switch {
		case ctx.Err() != nil:
			return errors.Wrapf(ctx.Err(), "waiting for client update tx %s", submission.TxHash)
		case errors.Is(err, v2.ErrTxNotFound):
			// still pending, poll again
			failingSince = time.Time{}
		case err != nil:
			if failingSince.IsZero() {
				failingSince = time.Now()
				r.logger.Warn("Checking client update tx failed, polling again",
					"txHash", submission.TxHash, "error", err)
			} else if time.Since(failingSince) >= updateTxLookupTimeout {
				return errors.Wrapf(err, "checking client update tx %s", submission.TxHash)
			}
		case retry:
			return errors.Errorf("client update tx %s reverted or was not mined in time", submission.TxHash)
		default:
			return nil
		}

		select {
		case <-ctx.Done():
			return errors.Wrapf(ctx.Err(), "waiting for client update tx %s", submission.TxHash)
		case <-ticker.C:
		}
	}
}

// validateRelaySimulation refuses a transaction in which a client update
// reports misbehavior, which would freeze the client, or in which no call of
// the kind the transaction exists for can succeed.
func validateRelaySimulation(ctx context.Context, logger *slog.Logger, simulation v2.RelaySimulation) error {
	if slices.ContainsFunc(simulation.Operations, func(result v2.OperationResult) bool {
		return result.Misbehavior
	}) {
		return errors.New("a client update would report misbehavior and freeze the client, not submitting it")
	}
	requiredType, requiredKind := v2.OperationTypeUpdate, "client update"
	if slices.ContainsFunc(simulation.Operations, func(result v2.OperationResult) bool {
		return result.Type == v2.OperationTypePacket
	}) {
		requiredType, requiredKind = v2.OperationTypePacket, "packet"
	}
	var failures []string
	canSucceed := false
	for i, result := range simulation.Operations {
		if result.Success {
			canSucceed = canSucceed || result.Type == requiredType
			continue
		}
		if len(failures) < 5 {
			reason := result.FailureReason
			if len(reason) > 256 {
				reason = reason[:256] + "..."
			}
			failures = append(failures, fmt.Sprintf("call[%d]: %s", i, reason))
		}
	}
	details := strings.Join(failures, "; ")
	if canSucceed {
		if len(failures) > 0 {
			logger.DebugContext(ctx, "Relay simulation has failed calls", "failures", details)
		}
		return nil
	}
	return errors.Errorf("no %s calls can currently succeed: %s", requiredKind, details)
}
