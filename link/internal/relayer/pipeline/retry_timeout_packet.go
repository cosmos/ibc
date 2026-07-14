//nolint:dupl // the retry directions are structurally parallel by design
package pipeline

import (
	"context"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/store"
	"github.com/cosmos/ibc/link/internal/txmgr"
)

// RetryTimeoutPacket clears a stuck or failed timeout tx so the timeout is
// redelivered on the next run.
type RetryTimeoutPacket struct {
	submitter txmgr.Submitter
	storage   ClearTxStorage
	route     Route
}

func NewRetryTimeoutPacket(submitter txmgr.Submitter, storage ClearTxStorage, route Route) RetryTimeoutPacket {
	return RetryTimeoutPacket{submitter: submitter, storage: storage, route: route}
}

func (p RetryTimeoutPacket) Process(ctx context.Context, transfer *Transfer) (*Transfer, error) {
	if transfer.TimeoutTxHash == nil || transfer.TimeoutTxTime == nil {
		return nil, errors.New("transfer has no timeout tx details, violates ShouldProcess")
	}

	retry, err := p.submitter.ShouldRetry(
		ctx,
		p.route.SourceChainID,
		*transfer.TimeoutTxHash,
		RetryTimeoutExpiry,
		*transfer.TimeoutTxTime,
	)
	if err != nil {
		return nil, errors.Wrapf(err, "checking if timeout tx %s should be retried", *transfer.TimeoutTxHash)
	}

	if !retry {
		return transfer, nil
	}

	if err := p.storage.ClearPacketTimeoutTx(ctx, transfer.Key()); err != nil {
		return nil, errors.Wrapf(err, "clearing timeout tx %s", *transfer.TimeoutTxHash)
	}

	return nil, ErrRetryingTimeoutPacket
}

func (p RetryTimeoutPacket) Cancel(transfer *Transfer, err error) {
	cancelRetry(transfer, err, ErrRetryingTimeoutPacket, "timeout")
}

func (p RetryTimeoutPacket) ShouldProcess(transfer *Transfer) bool {
	return transfer.TimeoutTxHash != nil && transfer.TimeoutTxTime != nil
}

func (p RetryTimeoutPacket) Status() store.RelayStatus {
	return store.RelayStatusDeliverTimeoutPacket
}
