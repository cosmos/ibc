package processors

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/relay/proofgen"
	"github.com/cosmos/ibc/link/internal/relay/txbuilder"
	"github.com/cosmos/ibc/link/internal/txsubmitter"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

// findPacketEvent returns the event among events matching sequence and clientID
func findPacketEvent(
	events []v2.PacketEvent,
	sequence uint64,
	clientID string,
	provableHeight uint64,
) (v2.PacketEvent, error) {
	for _, event := range events {
		if event.Packet.Sequence != sequence || event.Packet.SourceClient != clientID {
			continue
		}

		if event.Height > provableHeight {
			return v2.PacketEvent{}, errors.Errorf(
				"packet observed at height %d exceeds currently provable height %d", event.Height, provableHeight,
			)
		}

		return event, nil
	}

	return v2.PacketEvent{}, errors.Errorf("no packet event for sequence %d client %q", sequence, clientID)
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

// relayPackets generates a state proof and per-packet proofs for events at
// proofHeight, asks txBuilder for the resulting transaction, and submits it
// via txSubmitter
func relayPackets(
	ctx context.Context,
	chainClient chains.Client,
	proofGen proofgen.ProofGenerator,
	txBuilder txbuilder.TxBuilder,
	txSubmitter txsubmitter.TxSubmitter,
	clientID string,
	relayKind v2.RelayKind,
	proofHeight uint64,
	events []v2.PacketEvent,
) (*v2.Submission, error) {
	stateProof, err := proofGen.StateProof(ctx, proofHeight)
	if err != nil {
		return nil, errors.Wrap(err, "generating state proof")
	}

	packets := make([]channeltypesv2.Packet, len(events))
	for i, event := range events {
		packets[i] = event.Packet
	}

	packetProofs, err := proofGen.PacketProofs(ctx, proofHeight, proofKindFor(relayKind), packets)
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

	relayTxs, err := txBuilder.BuildRelayTxs(v2.ClientUpdate{
		ClientID:   clientID,
		StateProof: stateProof,
	}, items)
	if err != nil {
		return nil, errors.Wrap(err, "building relay tx")
	}

	if len(relayTxs) != 1 {
		return nil, errors.Errorf("expected exactly one relay tx, got %d", len(relayTxs))
	}

	relayTx := relayTxs[0]

	waitCtx, cancel := context.WithTimeout(ctx, waitForChainTimeout)
	defer cancel()

	if err = chainClient.WaitForChain(waitCtx); err != nil {
		return nil, errors.Wrap(err, "waiting for chain")
	}

	submission, err := txSubmitter.Submit(ctx, v2.TxIntent{
		To:   common.BytesToAddress(relayTx.To).Hex(),
		Data: relayTx.Data,
	})
	if err != nil {
		return nil, errors.Wrap(err, "submitting relay tx")
	}

	return submission, nil
}
