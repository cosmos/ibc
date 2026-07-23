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
// transfers.
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

	events := batchPacketEvents(ctx, destinationClient, transfers, txbuilder.KindAck, height)
	events = filterAcksPresent(events, transfers)
	if len(events) == 0 {
		return transfers, nil
	}

	client, ok := p.chains.Get(p.route.SourceChainID)
	if !ok {
		return nil, errors.Errorf("no configured chain client for chain %s", p.route.SourceChainID)
	}

	txBuilder, ok := p.txBuilders.Get(p.route.SourceChainID)
	if !ok {
		return nil, errors.Errorf("no tx builder configured for chain %s", p.route.SourceChainID)
	}

	relayTx, err := buildRelayTx(
		ctx, client, proofGen, txBuilder,
		p.route.SourceClientID, proofgen.KindAcknowledgement, txbuilder.KindAck,
		height, events,
	)
	if err != nil {
		return nil, err
	}

	submission, err := p.txManager.Submit(ctx, v2.TxIntent{
		To:   common.BytesToAddress(relayTx.To).Hex(),
		Data: relayTx.Data,
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
