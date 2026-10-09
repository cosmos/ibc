// SPDX-License-Identifier: Apache-2.0

package processors

import (
	"context"
	"log/slog"

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

// proofKindFor maps operationType to the proof claim it requires
func proofKindFor(operationType v2.OperationType) v2.ProofKind {
	switch operationType {
	case v2.OperationTypeRecv:
		return v2.ProofKindPacketCommitment
	case v2.OperationTypeAck:
		return v2.ProofKindAcknowledgement
	case v2.OperationTypeTimeout:
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
	sourceChainID string,
	operationType v2.OperationType,
	proofHeight uint64,
	events []v2.PacketEvent,
) error {
	sequences := make([]uint64, len(events))
	for i, event := range events {
		sequences[i] = event.Packet.Sequence
	}

	logger = logger.With(
		"kind",
		operationType,
		"clientID",
		clientID,
		"proofHeight",
		proofHeight,
		"sequences",
		sequences,
	)
	logger.Debug("Relaying packets")

	clientUpdatePayloads, err := prover.ClientUpdatePayloads(ctx, proofHeight)
	if err != nil {
		return errors.Wrap(err, "generating client update payloads")
	}

	packets := make([]channeltypesv2.Packet, len(events))
	var acknowledgements []channeltypesv2.Acknowledgement
	if operationType == v2.OperationTypeAck {
		acknowledgements = make([]channeltypesv2.Acknowledgement, len(events))
	}
	for i, event := range events {
		packets[i] = event.Packet
		if operationType == v2.OperationTypeAck {
			acknowledgements[i] = channeltypesv2.NewAcknowledgement(event.Acks...)
		}
	}

	packetProofs, err := prover.PacketProofs(ctx, proofHeight, proofKindFor(operationType), packets, acknowledgements)
	if err != nil {
		return errors.Wrap(err, "generating packet proofs")
	}

	items := make([]v2.PacketRelayItem, len(events))
	for i, event := range events {
		items[i] = v2.PacketRelayItem{
			Type:        operationType,
			Packet:      event.Packet,
			Acks:        event.Acks,
			Proof:       packetProofs[i],
			ProofHeight: proofHeight,
		}
	}

	waitCtx, cancel := context.WithTimeout(ctx, waitForChainTimeout)
	defer cancel()
	if err = chainClient.WaitForChain(waitCtx); err != nil {
		return errors.Wrap(err, "waiting for chain")
	}

	relayTxs, err := txBuilder.BuildRelayTxs(v2.ClientUpdate{ClientID: clientID, Payloads: clientUpdatePayloads}, items)
	if err != nil {
		return errors.Wrap(err, "building relay tx")
	}
	if len(relayTxs) != 1 {
		return errors.Errorf("expected exactly one relay tx, got %d", len(relayTxs))
	}
	operations := make([]v2.OperationMetadata, len(items))
	for i, item := range items {
		operations[i] = v2.OperationMetadata{
			SourceChainID:  sourceChainID,
			SourceClientID: item.Packet.SourceClient,
			Sequence:       item.Packet.Sequence,
			Type:           operationType,
		}
	}
	err = txSubmitter.Submit(ctx, v2.TxIntent{
		To:         common.BytesToAddress(relayTxs[0].To).Hex(),
		Data:       relayTxs[0].Data,
		Operations: operations,
	})
	if err != nil {
		return errors.Wrap(err, "submitting relay tx")
	}
	return nil
}
