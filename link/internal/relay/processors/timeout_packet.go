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

// BatchTimeoutPacket delivers one timeout tx on the source chain for a batch
// of transfers, proving non-membership of the packet's receipt on the
// destination chain via the source chain's client tracking it.
type BatchTimeoutPacket struct {
	chains          ChainClients
	storage         TxStorage
	proofGenerators ProofGenerators
	txBuilders      TxBuilders
	txManager       txmgr.TxManager
	route           Route
}

func NewBatchTimeoutPacket(
	chainClients ChainClients,
	storage TxStorage,
	proofGenerators ProofGenerators,
	txBuilders TxBuilders,
	txManager txmgr.TxManager,
	route Route,
) BatchTimeoutPacket {
	return BatchTimeoutPacket{
		chains:          chainClients,
		storage:         storage,
		proofGenerators: proofGenerators,
		txBuilders:      txBuilders,
		txManager:       txManager,
		route:           route,
	}
}

func (p BatchTimeoutPacket) Process(ctx context.Context, transfers []*Transfer) ([]*Transfer, error) {
	proofGen, ok := p.proofGenerators.Get(p.route.SourceChainID, p.route.SourceClientID)
	if !ok {
		return nil, errors.Errorf(
			"no proof generator configured for client %q on chain %q", p.route.SourceClientID, p.route.SourceChainID,
		)
	}

	height, timestamp, err := proofGen.LatestProvableHeight(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "resolving latest provable height")
	}

	client, ok := p.chains.Get(p.route.SourceChainID)
	if !ok {
		return nil, errors.Errorf("no configured chain client for chain %s", p.route.SourceChainID)
	}

	events := batchPacketEvents(ctx, client, transfers, txbuilder.KindTimeout, height)

	events = filterProvableTimeouts(events, timestamp, transfers)
	if len(events) == 0 {
		return transfers, nil
	}

	txBuilder, ok := p.txBuilders.Get(p.route.SourceChainID)
	if !ok {
		return nil, errors.Errorf("no tx builder configured for chain %s", p.route.SourceChainID)
	}

	relayTx, err := buildRelayTx(
		ctx, client, proofGen, txBuilder,
		p.route.SourceClientID, proofgen.KindReceiptAbsence, txbuilder.KindTimeout,
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

			if errRecord := repo.UpdatePacketTimeoutTx(ctx, tr.Key(), tx); errRecord != nil {
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

		tr.TimeoutTxHash = &tx.Hash
		tr.TimeoutTxTime = &tx.Time
		tr.TimeoutTxRelayerAddress = &tx.RelayerAddress
	}

	return transfers, nil
}

func (p BatchTimeoutPacket) Cancel(transfers []*Transfer, err error) {
	for _, tr := range transfers {
		tr.GetLogger().Error("Delivering batch timeout tx", "error", err)
	}
}

func (p BatchTimeoutPacket) ShouldProcess(tr *Transfer) bool {
	shouldBeTimedOut := tr.IsTimedOut() && tr.RecvTxHash == nil && tr.AckTxHash == nil

	return shouldBeTimedOut && tr.TimeoutTxHash == nil
}

func (p BatchTimeoutPacket) Status() store.RelayStatus {
	return store.RelayStatusDeliverTimeoutPacket
}
