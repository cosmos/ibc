package processors

import (
	"context"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/relayer/transfer"
	"github.com/cosmos/ibc/link/internal/store"
	"github.com/cosmos/ibc/link/internal/txmgr"
)

// RetryRecvPacket clears a stuck or failed recv tx so the packet is
// redelivered on the next run.
type RetryRecvPacket struct {
	submitter txmgr.Submitter
	storage   ClearTxStorage
	route     transfer.Route
}

func NewRetryRecvPacket(submitter txmgr.Submitter, storage ClearTxStorage, route transfer.Route) RetryRecvPacket {
	return RetryRecvPacket{submitter: submitter, storage: storage, route: route}
}

func (p RetryRecvPacket) Process(ctx context.Context, tr *transfer.Transfer) (*transfer.Transfer, error) {
	if tr.RecvTxHash == nil || tr.RecvTxTime == nil {
		return nil, errors.New("transfer has no recv tx details, violates ShouldProcess")
	}

	retry, err := p.submitter.ShouldRetry(
		ctx,
		p.route.DestinationChainID,
		*tr.RecvTxHash,
		RetryRecvExpiry,
		*tr.RecvTxTime,
	)
	if err != nil {
		return nil, errors.Wrapf(err, "checking if recv tx %s should be retried", *tr.RecvTxHash)
	}

	if !retry {
		return tr, nil
	}

	if err := p.storage.ClearPacketRecvTx(ctx, tr.Key()); err != nil {
		return nil, errors.Wrapf(err, "clearing recv tx %s", *tr.RecvTxHash)
	}

	// error so the tr stops processing this run; it is picked up
	// without the recv tx and redelivered on the next run
	return nil, transfer.ErrRetryingRecvPacket
}

func (p RetryRecvPacket) Cancel(tr *transfer.Transfer, err error) {
	cancelRetry(tr, err, transfer.ErrRetryingRecvPacket, "recv")
}

func (p RetryRecvPacket) ShouldProcess(tr *transfer.Transfer) bool {
	return tr.RecvTxHash != nil && tr.RecvTxTime != nil && tr.WriteAckTxHash == nil
}

func (p RetryRecvPacket) Status() store.RelayStatus {
	return store.RelayStatusDeliverRecvPacket
}
