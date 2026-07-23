package processors

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/relay/proofgen"
	"github.com/cosmos/ibc/link/internal/relay/txbuilder"
	"github.com/cosmos/ibc/link/internal/store"
	"github.com/cosmos/ibc/link/internal/txmgr"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

// BatchAckPacket delivers one ack tx on the source chain for a batch of
// transfers. Acks flow back toward the original source chain, so the
// destination client processing them is the source chain's client tracking
// the destination chain.
type BatchAckPacket struct {
	chains          ChainClients
	storage         TxStorage
	proofGenerators ProofGenerators
	txBuilders      TxBuilders
	txManager       txmgr.TxManager
	route           Route
}

func NewBatchAckPacket(
	chainClients ChainClients,
	storage TxStorage,
	proofGenerators ProofGenerators,
	txBuilders TxBuilders,
	txManager txmgr.TxManager,
	route Route,
) BatchAckPacket {
	return BatchAckPacket{
		chains:          chainClients,
		storage:         storage,
		proofGenerators: proofGenerators,
		txBuilders:      txBuilders,
		txManager:       txManager,
		route:           route,
	}
}

func (p BatchAckPacket) Process(ctx context.Context, transfers []*Transfer) ([]*Transfer, error) {
	proofGen, ok := p.proofGenerators.Get(p.route.SourceChainID, p.route.SourceClientID)
	if !ok {
		return nil, errors.Errorf(
			"no proof generator configured for client %q on chain %q", p.route.SourceClientID, p.route.SourceChainID,
		)
	}

	height, _, err := proofGen.LatestProvableHeight(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "resolving latest provable height")
	}

	destinationClient, ok := p.chains.Get(p.route.DestinationChainID)
	if !ok {
		return nil, errors.Errorf("no configured chain client for chain %s", p.route.DestinationChainID)
	}

	events := batchPacketEvents(ctx, destinationClient, transfers, func(tr *Transfer) (string, bool) {
		if tr.WriteAckTxHash == nil {
			return "", false
		}

		return *tr.WriteAckTxHash, true
	})

	events = filterProvableEvents(events, height, transfers)
	if len(events) == 0 {
		return transfers, nil
	}

	stateProof, err := proofGen.StateProof(ctx, height)
	if err != nil {
		return nil, errors.Wrap(err, "generating state proof")
	}

	packets := make([]v2.Packet, len(events))
	for i, event := range events {
		packets[i] = event.Packet
	}

	packetProofs, err := proofGen.PacketProofs(ctx, height, proofgen.KindAcknowledgement, packets)
	if err != nil {
		return nil, errors.Wrap(err, "generating packet proofs")
	}

	bySeq := transfersBySequence(transfers)

	items := make([]txbuilder.PacketRelayItem, 0, len(events))

	for i, event := range events {
		if len(event.Acks) == 0 {
			if tr, ok := bySeq[event.Packet.Sequence]; ok {
				tr.ProcessingError = errors.Errorf("no acknowledgement recorded for sequence %d", event.Packet.Sequence)
			}

			continue
		}

		items = append(items, txbuilder.PacketRelayItem{
			Kind:        txbuilder.KindAck,
			Packet:      event.Packet,
			Acks:        event.Acks,
			Proof:       packetProofs[i],
			ProofHeight: height,
		})
	}

	if len(items) == 0 {
		return transfers, nil
	}

	txBuilder, ok := p.txBuilders.Get(p.route.SourceChainID)
	if !ok {
		return nil, errors.Errorf("no tx builder configured for chain %s", p.route.SourceChainID)
	}

	relayTxs, err := txBuilder.BuildRelayTxs(txbuilder.ClientUpdate{
		ClientID:   p.route.SourceClientID,
		StateProof: stateProof,
	}, items)
	if err != nil {
		return nil, errors.Wrap(err, "building relay tx")
	}

	if len(relayTxs) != 1 {
		return nil, errors.Errorf("expected exactly one relay tx, got %d", len(relayTxs))
	}

	client, ok := p.chains.Get(p.route.SourceChainID)
	if !ok {
		return nil, errors.Errorf("no configured chain client for chain %s", p.route.SourceChainID)
	}

	// the chain must be caught up to the current time before gas estimation
	// during delivery, or the tx reverts
	waitCtx, cancel := context.WithTimeout(ctx, waitForChainTimeout)
	defer cancel()

	if errWait := client.WaitForChain(waitCtx); errWait != nil {
		return nil, errors.Wrap(errWait, "waiting for chain")
	}

	submission, err := p.txManager.Submit(ctx, v2.TxIntent{
		To:   common.BytesToAddress(relayTxs[0].To).Hex(),
		Data: relayTxs[0].Data,
	})
	if err != nil {
		return nil, errors.Wrap(err, "submitting relay tx")
	}

	tx := store.PacketTx{
		Hash:           submission.TxHash,
		Time:           submission.SubmittedAt,
		RelayerAddress: submission.RelayerAddress,
	}

	err = p.storage.Transact(ctx, func(repo store.Repository) error {
		for _, tr := range transfers {
			if tr.ProcessingError != nil {
				continue
			}

			if errRecord := repo.UpdatePacketAckTx(ctx, tr.Key(), tx); errRecord != nil {
				return errors.Wrapf(
					errRecord,
					"recording relay tx %s for sequence %d",
					tx.Hash,
					tr.PacketSequenceNumber,
				)
			}
		}

		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "recording batch relay txs")
	}

	for _, tr := range transfers {
		if tr.ProcessingError != nil {
			continue
		}

		tr.AckTxHash = &tx.Hash
		tr.AckTxTime = &tx.Time
		tr.AckTxRelayerAddress = &tx.RelayerAddress
	}

	return transfers, nil
}

func (p BatchAckPacket) Cancel(transfers []*Transfer, err error) {
	for _, tr := range transfers {
		tr.GetLogger().Error("Delivering batch ack tx", "error", err)
	}
}

func (p BatchAckPacket) ShouldProcess(tr *Transfer) bool {
	if tr.WriteAckTxHash == nil {
		return false
	}

	return tr.AckTxHash == nil && tr.TimeoutTxHash == nil
}

func (p BatchAckPacket) Status() store.RelayStatus {
	return store.RelayStatusDeliverAckPacket
}
