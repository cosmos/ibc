package processors

import (
	"context"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/relayer/transfer"
	"github.com/cosmos/ibc/link/internal/store"
	"github.com/cosmos/ibc/link/internal/txmgr"

	proto "github.com/cosmos/ibc/link/internal/types/proofapi"
)

// BatchTimeoutPacket delivers one timeout tx on the source chain for a batch
// of transfers.
type BatchTimeoutPacket struct {
	batchDeps
}

func NewBatchTimeoutPacket(
	chainClients ChainClients,
	storage TxStorage,
	proofAPI proto.ProofApiServiceClient,
	submitter txmgr.Submitter,
	route transfer.Route,
) BatchTimeoutPacket {
	return BatchTimeoutPacket{
		batchDeps{chains: chainClients, storage: storage, proofAPI: proofAPI, submitter: submitter, route: route},
	}
}

//nolint:dupl // the batch directions are structurally parallel by design
func (p BatchTimeoutPacket) Process(ctx context.Context, transfers []*transfer.Transfer) ([]*transfer.Transfer, error) {
	txIDs, sequences := collectTxIDs(transfers, func(t *transfer.Transfer) (string, error) {
		return t.SourceTxHash, nil
	})

	// timeouts flow back toward the original source chain, so the proof api's
	// source and destination are inverted
	out, err := p.processBatch(ctx, transfers, &proto.RelayByTxRequest{
		SrcChain:           p.route.DestinationChainID,
		DstChain:           p.route.SourceChainID,
		TimeoutTxIds:       txIDs,
		SrcClientId:        p.route.DestinationClientID,
		DstClientId:        p.route.SourceClientID,
		DstPacketSequences: sequences,
	}, p.route.SourceChainID,
		func(repo store.Repository, key store.PacketKey, tx store.PacketTx) error {
			return repo.UpdatePacketTimeoutTx(ctx, key, tx)
		},
		func(tr *transfer.Transfer, tx store.PacketTx) {
			tr.TimeoutTxHash = &tx.Hash
			tr.TimeoutTxTime = &tx.Time
			tr.TimeoutTxRelayerAddress = &tx.RelayerAddress
		})
	if err != nil {
		return nil, errors.Wrapf(err, "delivering timeout tx for batch of %d transfers", len(sequences))
	}

	return out, nil
}

func (p BatchTimeoutPacket) Cancel(transfers []*transfer.Transfer, err error) {
	for _, tr := range transfers {
		tr.GetLogger().Error("Delivering batch timeout tx", "error", err)
	}
}

func (p BatchTimeoutPacket) ShouldProcess(tr *transfer.Transfer) bool {
	shouldBeTimedOut := tr.IsTimedOut() && tr.RecvTxHash == nil && tr.AckTxHash == nil

	return shouldBeTimedOut && tr.TimeoutTxHash == nil
}

func (p BatchTimeoutPacket) Status() store.RelayStatus {
	return store.RelayStatusDeliverTimeoutPacket
}
