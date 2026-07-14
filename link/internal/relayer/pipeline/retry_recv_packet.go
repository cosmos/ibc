package pipeline

import (
	"context"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/store"
	"github.com/cosmos/ibc/link/internal/txmgr"
)

// RetryRecvPacket clears a stuck or failed recv tx so the packet is
// redelivered on the next run.
type RetryRecvPacket struct {
	submitter txmgr.Submitter
	storage   ClearTxStorage
	route     Route
}

func NewRetryRecvPacket(submitter txmgr.Submitter, storage ClearTxStorage, route Route) RetryRecvPacket {
	return RetryRecvPacket{submitter: submitter, storage: storage, route: route}
}

func (p RetryRecvPacket) Process(ctx context.Context, transfer *Transfer) (*Transfer, error) {
	if transfer.RecvTxHash == nil || transfer.RecvTxTime == nil {
		return nil, errors.New("transfer has no recv tx details, violates ShouldProcess")
	}

	retry, err := p.submitter.ShouldRetry(
		ctx,
		p.route.DestinationChainID,
		*transfer.RecvTxHash,
		RetryRecvExpiry,
		*transfer.RecvTxTime,
	)
	if err != nil {
		return nil, errors.Wrapf(err, "checking if recv tx %s should be retried", *transfer.RecvTxHash)
	}

	if !retry {
		return transfer, nil
	}

	if err := p.storage.ClearPacketRecvTx(ctx, transfer.Key()); err != nil {
		return nil, errors.Wrapf(err, "clearing recv tx %s", *transfer.RecvTxHash)
	}

	// error so the transfer stops processing this run; it is picked up
	// without the recv tx and redelivered on the next run
	return nil, ErrRetryingRecvPacket
}

func (p RetryRecvPacket) Cancel(transfer *Transfer, err error) {
	cancelRetry(transfer, err, ErrRetryingRecvPacket, "recv")
}

func (p RetryRecvPacket) ShouldProcess(transfer *Transfer) bool {
	return transfer.RecvTxHash != nil && transfer.RecvTxTime != nil && transfer.WriteAckTxHash == nil
}

func (p RetryRecvPacket) Status() store.RelayStatus {
	return store.RelayStatusDeliverRecvPacket
}
