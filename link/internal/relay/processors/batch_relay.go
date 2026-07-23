package processors

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/relay/proofgen"
	"github.com/cosmos/ibc/link/internal/relay/txbuilder"
	"github.com/cosmos/ibc/link/internal/txmgr"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

// findPacketEvent returns the event among events matching sequence and
// clientID -- the packet's identity, since sequence numbers alone aren't
// unique across different client pairs sharing one chain's events -- if
// it's been observed at or below height, the height a membership/
// non-membership proof can currently be generated for.
func findPacketEvent(events []v2.PacketEvent, sequence uint64, clientID string, provableHeight uint64) (v2.PacketEvent, error) {
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

// proofKindFor maps relayKind to the proof claim it requires: recv proves a
// packet commitment exists on the source chain, ack proves an
// acknowledgement exists on the destination chain, and timeout proves a
// packet receipt is absent from the destination chain.
func proofKindFor(relayKind txbuilder.RelayKind) proofgen.ProofKind {
	switch relayKind {
	case txbuilder.KindRecv:
		return proofgen.KindPacketCommitment
	case txbuilder.KindAck:
		return proofgen.KindAcknowledgement
	case txbuilder.KindTimeout:
		return proofgen.KindReceiptAbsence
	default:
		return proofgen.KindUnknown
	}
}

// relayPackets generates a state proof and per-packet proofs for events at
// proofHeight, packs them into relayKind-tagged items (order-preserved, one
// per event), asks txBuilder for the resulting transaction, waits for
// chainClient's chain to catch up to the current time (gas estimation during
// submission reverts otherwise), and submits it via txManager. clientID is
// the destination client the state proof updates.
func relayPackets(
	ctx context.Context,
	chainClient chains.Client,
	proofGen proofgen.ProofGenerator,
	txBuilder txbuilder.TxBuilder,
	txManager txmgr.TxManager,
	clientID string,
	relayKind txbuilder.RelayKind,
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

	items := make([]txbuilder.PacketRelayItem, len(events))
	for i, event := range events {
		items[i] = txbuilder.PacketRelayItem{
			Kind:        relayKind,
			Packet:      event.Packet,
			Acks:        event.Acks,
			Proof:       packetProofs[i],
			ProofHeight: proofHeight,
		}
	}

	relayTxs, err := txBuilder.BuildRelayTxs(txbuilder.ClientUpdate{
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

	if err := chainClient.WaitForChain(waitCtx); err != nil {
		return nil, errors.Wrap(err, "waiting for chain")
	}

	submission, err := txManager.Submit(ctx, v2.TxIntent{
		To:   common.BytesToAddress(relayTx.To).Hex(),
		Data: relayTx.Data,
	})
	if err != nil {
		return nil, errors.Wrap(err, "submitting relay tx")
	}

	return submission, nil
}
