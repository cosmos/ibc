package pipeline

import (
	"context"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/store"
	"github.com/cosmos/ibc/link/internal/txmgr"

	proto "github.com/cosmos/ibc/link/internal/types/proofapi"
)

// BatchRecvPacket delivers one recv tx on the destination chain for a batch
// of transfers.
type BatchRecvPacket struct {
	batchDeps
}

func NewBatchRecvPacket(
	chainClients ChainClients,
	storage TxStorage,
	proofAPI proto.ProofApiServiceClient,
	submitter txmgr.Submitter,
	route Route,
) BatchRecvPacket {
	return BatchRecvPacket{
		batchDeps{chains: chainClients, storage: storage, proofAPI: proofAPI, submitter: submitter, route: route},
	}
}

//nolint:dupl // the batch directions are structurally parallel by design
func (p BatchRecvPacket) Process(ctx context.Context, transfers []*Transfer) ([]*Transfer, error) {
	txIDs, sequences := collectTxIDs(transfers, func(t *Transfer) (string, error) {
		return t.SourceTxHash, nil
	})

	out, err := p.processBatch(ctx, transfers, &proto.RelayByTxRequest{
		SrcChain:           p.route.SourceChainID,
		DstChain:           p.route.DestinationChainID,
		SourceTxIds:        txIDs,
		SrcClientId:        p.route.SourceClientID,
		DstClientId:        p.route.DestinationClientID,
		SrcPacketSequences: sequences,
	}, p.route.DestinationChainID,
		func(repo store.Repository, key store.PacketKey, tx store.PacketTx) error {
			return repo.UpdatePacketRecvTx(ctx, key, tx)
		},
		func(transfer *Transfer, tx store.PacketTx) {
			transfer.RecvTxHash = &tx.Hash
			transfer.RecvTxTime = &tx.Time
			transfer.RecvTxRelayerAddress = &tx.RelayerAddress
		})
	if err != nil {
		return nil, errors.Wrapf(err, "delivering recv tx for batch of %d transfers", len(sequences))
	}

	return out, nil
}

func (p BatchRecvPacket) Cancel(transfers []*Transfer, err error) {
	for _, transfer := range transfers {
		transfer.GetLogger().Error("Delivering batch recv tx", "error", err)
	}
}

func (p BatchRecvPacket) ShouldProcess(transfer *Transfer) bool {
	return transfer.RecvTxHash == nil && !transfer.IsTimedOut()
}

func (p BatchRecvPacket) Status() store.RelayStatus {
	return store.RelayStatusDeliverRecvPacket
}
