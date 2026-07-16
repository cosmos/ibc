//nolint:dupl // the retry directions are structurally parallel by design
package processors

import (
	"context"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/relayer/transfer"
	"github.com/cosmos/ibc/link/internal/store"
	"github.com/cosmos/ibc/link/internal/txmgr"
)

// RetryTimeoutPacket clears a stuck or failed timeout tx so the timeout is
// redelivered on the next run.
type RetryTimeoutPacket struct {
	submitter txmgr.Submitter
	storage   ClearTxStorage
	route     transfer.Route
}

func NewRetryTimeoutPacket(submitter txmgr.Submitter, storage ClearTxStorage, route transfer.Route) RetryTimeoutPacket {
	return RetryTimeoutPacket{submitter: submitter, storage: storage, route: route}
}

func (p RetryTimeoutPacket) Process(ctx context.Context, tr *transfer.Transfer) (*transfer.Transfer, error) {
	if tr.TimeoutTxHash == nil || tr.TimeoutTxTime == nil {
		return nil, errors.New("transfer has no timeout tx details, violates ShouldProcess")
	}

	retry, err := p.submitter.ShouldRetry(
		ctx,
		p.route.SourceChainID,
		*tr.TimeoutTxHash,
		RetryTimeoutExpiry,
		*tr.TimeoutTxTime,
	)
	if err != nil {
		return nil, errors.Wrapf(err, "checking if timeout tx %s should be retried", *tr.TimeoutTxHash)
	}

	if !retry {
		return tr, nil
	}

	if err := p.storage.ClearPacketTimeoutTx(ctx, tr.Key()); err != nil {
		return nil, errors.Wrapf(err, "clearing timeout tx %s", *tr.TimeoutTxHash)
	}

	return nil, transfer.ErrRetryingTimeoutPacket
}

func (p RetryTimeoutPacket) Cancel(tr *transfer.Transfer, err error) {
	cancelRetry(tr, err, transfer.ErrRetryingTimeoutPacket, "timeout")
}

func (p RetryTimeoutPacket) ShouldProcess(tr *transfer.Transfer) bool {
	return tr.TimeoutTxHash != nil && tr.TimeoutTxTime != nil
}

func (p RetryTimeoutPacket) Status() store.RelayStatus {
	return store.RelayStatusDeliverTimeoutPacket
}
