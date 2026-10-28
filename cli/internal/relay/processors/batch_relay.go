// SPDX-License-Identifier: Apache-2.0

package processors

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"

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

// relayPackets generates a client update payload and per-packet proofs for events at
// proofHeight, asks txBuilder for the resulting transaction, and submits it
// via txSubmitter
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

	relayTxs, err := txBuilder.BuildRelayTxs(v2.ClientUpdate{
		ClientID: clientID,
		Payloads: clientUpdatePayloads,
	}, items)
	if err != nil {
		return nil, errors.Wrap(err, "building relay tx")
	}

	if len(relayTxs) != 1 {
		return nil, errors.Errorf("expected exactly one relay tx, got %d", len(relayTxs))
	}

	relayTx := relayTxs[0]
	simulation, err := txSubmitter.SimulateRelayTx(ctx, relayTx)
	if err != nil {
		return nil, errors.Wrap(err, "simulating relay tx")
	}
	if err = validateRelaySimulation(ctx, logger, simulation); err != nil {
		return nil, err
	}

	submission, err := txSubmitter.Submit(ctx, v2.TxIntent{
		To:   common.BytesToAddress(relayTx.To).Hex(),
		Data: relayTx.Data,
	})
	if err != nil {
		return nil, errors.Wrap(err, "submitting relay tx")
	}

	chainID := chainClient.ChainID()
	logger.Info("Relay transaction submitted", "chainID", chainID, "txHash", submission.TxHash)

	metrics.txSubmitted(ctx, chainID, clientID, submission.TxHash)

	return submission, nil
}

func validateRelaySimulation(ctx context.Context, logger *slog.Logger, simulation v2.RelaySimulation) error {
	requiredType := v2.OperationTypeUpdate
	if slices.ContainsFunc(simulation.Operations, func(result v2.OperationResult) bool {
		return result.Type == v2.OperationTypePacket
	}) {
		requiredType = v2.OperationTypePacket
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
	return errors.Errorf("no packet calls can currently succeed: %s", details)
}
