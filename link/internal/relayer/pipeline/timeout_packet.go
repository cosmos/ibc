package pipeline

import (
	"context"

	"github.com/pkg/errors"

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
	route Route,
) BatchTimeoutPacket {
	return BatchTimeoutPacket{
		batchDeps{chains: chainClients, storage: storage, proofAPI: proofAPI, submitter: submitter, route: route},
	}
}

//nolint:dupl // the batch directions are structurally parallel by design
func (p BatchTimeoutPacket) Process(ctx context.Context, transfers []*Transfer) ([]*Transfer, error) {
	txIDs, sequences := collectTxIDs(transfers, func(t *Transfer) (string, error) {
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
		func(transfer *Transfer, tx store.PacketTx) {
			transfer.TimeoutTxHash = &tx.Hash
			transfer.TimeoutTxTime = &tx.Time
			transfer.TimeoutTxRelayerAddress = &tx.RelayerAddress
		})
	if err != nil {
		return nil, errors.Wrapf(err, "delivering timeout tx for batch of %d transfers", len(sequences))
	}

	return out, nil
}

func (p BatchTimeoutPacket) Cancel(transfers []*Transfer, err error) {
	for _, transfer := range transfers {
		transfer.GetLogger().Error("Delivering batch timeout tx", "error", err)
	}
}

func (p BatchTimeoutPacket) ShouldProcess(transfer *Transfer) bool {
	shouldBeTimedOut := transfer.IsTimedOut() && transfer.RecvTxHash == nil && transfer.AckTxHash == nil

	return shouldBeTimedOut && transfer.TimeoutTxHash == nil
}

func (p BatchTimeoutPacket) Status() store.RelayStatus {
	return store.RelayStatusDeliverTimeoutPacket
}
