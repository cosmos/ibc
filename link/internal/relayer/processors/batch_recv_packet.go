package processors

import (
	"context"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/relayer/transfer"
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
	route transfer.Route,
) BatchRecvPacket {
	return BatchRecvPacket{
		batchDeps{chains: chainClients, storage: storage, proofAPI: proofAPI, submitter: submitter, route: route},
	}
}

//nolint:dupl // the batch directions are structurally parallel by design
func (p BatchRecvPacket) Process(ctx context.Context, transfers []*transfer.Transfer) ([]*transfer.Transfer, error) {
	txIDs, sequences := collectTxIDs(transfers, func(t *transfer.Transfer) (string, error) {
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
		func(tr *transfer.Transfer, tx store.PacketTx) {
			tr.RecvTxHash = &tx.Hash
			tr.RecvTxTime = &tx.Time
			tr.RecvTxRelayerAddress = &tx.RelayerAddress
		})
	if err != nil {
		return nil, errors.Wrapf(err, "delivering recv tx for batch of %d transfers", len(sequences))
	}

	return out, nil
}

func (p BatchRecvPacket) Cancel(transfers []*transfer.Transfer, err error) {
	for _, tr := range transfers {
		tr.GetLogger().Error("Delivering batch recv tx", "error", err)
	}
}

func (p BatchRecvPacket) ShouldProcess(tr *transfer.Transfer) bool {
	return tr.RecvTxHash == nil && !tr.IsTimedOut()
}

func (p BatchRecvPacket) Status() store.RelayStatus {
	return store.RelayStatusDeliverRecvPacket
}
