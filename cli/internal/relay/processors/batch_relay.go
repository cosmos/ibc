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

// relayPackets prepares proofs for events at proofHeight, confirms any
// client-only checkpoints the prover requires first, then submits one
// transaction carrying the final client update and every packet.
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

	chainID := chainClient.ChainID()
	unlock, err := lockClient(ctx, chainID, clientID)
	if err != nil {
		return nil, err
	}
	defer unlock()

	packets := make([]channeltypesv2.Packet, len(events))
	for i, event := range events {
		packets[i] = event.Packet
	}
	submit := func(update []byte, items []v2.PacketRelayItem) (*v2.Submission, error) {
		tx, err := txBuilder.BuildRelayTx(v2.ClientUpdate{ClientID: clientID, Proof: update}, items)
		if err != nil {
			return nil, errors.Wrap(err, "building relay tx")
		}

		waitCtx, cancel := context.WithTimeout(ctx, waitForChainTimeout)
		defer cancel()
		if err := chainClient.WaitForChain(waitCtx); err != nil {
			return nil, errors.Wrap(err, "waiting for chain")
		}
		return txSubmitter.Submit(ctx, v2.TxIntent{To: common.BytesToAddress(tx.To).Hex(), Data: tx.Data})
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		prepared, err := prover.Prepare(ctx, proofHeight, proofKindFor(relayKind), packets)
		if err != nil {
			return nil, errors.Wrap(err, "preparing relay proofs")
		}
		if err := prepared.Validate(len(packets)); err != nil {
			return nil, err
		}
		if prepared.Ready == nil {
			// Prepare reads confirmed on-chain state again after the checkpoint lands.
			sub, err := submit(prepared.Advance, nil)
			if err != nil {
				return nil, errors.Wrap(err, "submitting client checkpoint")
			}
			logger.Info("Submitted client checkpoint", "txHash", sub.TxHash)
			if err := confirmClientUpdate(ctx, txSubmitter, *sub); err != nil {
				return nil, err
			}
			continue
		}

		items := make([]v2.PacketRelayItem, len(events))
		for i, event := range events {
			items[i] = v2.PacketRelayItem{
				Kind: relayKind, Packet: event.Packet, Acks: event.Acks,
				Proof: prepared.Ready.PacketProofs[i], ProofHeight: proofHeight,
			}
		}
		sub, err := submit(prepared.Ready.Update, items)
		if err != nil {
			return nil, errors.Wrap(err, "submitting relay tx")
		}
		logger.Info("Relayed packets", "txHash", sub.TxHash)
		metrics.txSubmitted(ctx, chainID, clientID, sub.TxHash)
		return sub, nil
	}
}
