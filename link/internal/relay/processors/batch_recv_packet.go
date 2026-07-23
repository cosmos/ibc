//nolint:dupl // the batch directions are structurally parallel by design
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

// BatchRecvPacket delivers one recv tx on the destination chain for a batch
// of transfers.
type BatchRecvPacket struct {
	chains          ChainClients
	storage         TxStorage
	proofGenerators ProofGenerators
	txBuilders      TxBuilders
	txManager       txmgr.TxManager
	route           Route
}

func NewBatchRecvPacket(
	chainClients ChainClients,
	storage TxStorage,
	proofGenerators ProofGenerators,
	txBuilders TxBuilders,
	txManager txmgr.TxManager,
	route Route,
) BatchRecvPacket {
	return BatchRecvPacket{
		chains:          chainClients,
		storage:         storage,
		proofGenerators: proofGenerators,
		txBuilders:      txBuilders,
		txManager:       txManager,
		route:           route,
	}
}

func (p BatchRecvPacket) Process(ctx context.Context, transfers []*Transfer) ([]*Transfer, error) {
	proofGen, ok := p.proofGenerators.Get(p.route.DestinationChainID, p.route.DestinationClientID)
	if !ok {
		return nil, errors.Errorf(
			"no proof generator configured for client %q on chain %q", p.route.DestinationClientID, p.route.DestinationChainID,
		)
	}

	height, _, err := proofGen.LatestProvableHeight(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "resolving latest provable height")
	}

	sourceClient, ok := p.chains.Get(p.route.SourceChainID)
	if !ok {
		return nil, errors.Errorf("no configured chain client for chain %s", p.route.SourceChainID)
	}

	events := batchPacketEvents(ctx, sourceClient, transfers, txbuilder.KindRecv, height)
	if len(events) == 0 {
		return transfers, nil
	}

	destinationClient, ok := p.chains.Get(p.route.DestinationChainID)
	if !ok {
		return nil, errors.Errorf("no configured chain client for chain %s", p.route.DestinationChainID)
	}

	txBuilder, ok := p.txBuilders.Get(p.route.DestinationChainID)
	if !ok {
		return nil, errors.Errorf("no tx builder configured for chain %s", p.route.DestinationChainID)
	}

	relayTx, err := buildRelayTx(
		ctx, destinationClient, proofGen, txBuilder,
		p.route.DestinationClientID, proofgen.KindPacketCommitment, txbuilder.KindRecv,
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

			if errRecord := repo.UpdatePacketRecvTx(ctx, tr.Key(), tx); errRecord != nil {
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

		tr.RecvTxHash = &tx.Hash
		tr.RecvTxTime = &tx.Time
		tr.RecvTxRelayerAddress = &tx.RelayerAddress
	}

	return transfers, nil
}

func (p BatchRecvPacket) Cancel(transfers []*Transfer, err error) {
	for _, tr := range transfers {
		tr.GetLogger().Error("Delivering batch recv tx", "error", err)
	}
}

func (p BatchRecvPacket) ShouldProcess(tr *Transfer) bool {
	return tr.RecvTxHash == nil && !tr.IsTimedOut() && tr.AckTxHash == nil && tr.TimeoutTxHash == nil
}

func (p BatchRecvPacket) Status() store.RelayStatus {
	return store.RelayStatusDeliverRecvPacket
}
