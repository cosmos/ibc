//nolint:dupl // the retry directions are structurally parallel by design
package processors

import (
	"context"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/store"
	"github.com/cosmos/ibc/link/internal/txsubmitter"

	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

// ClearTimeoutTxStorage clears a recorded relay tx so it is resubmitted.
type ClearTimeoutTxStorage interface {
	ClearPacketTimeoutTx(ctx context.Context, key store.PacketKey) error
}

// RetryTimeoutPacket clears a stuck or failed timeout tx so the timeout is
// redelivered on the next run.
type RetryTimeoutPacket struct {
	txSubmitter txsubmitter.TxSubmitter
	storage     ClearTimeoutTxStorage
	route       Route
}

func NewRetryTimeoutPacket(
	txSubmitter txsubmitter.TxSubmitter,
	storage ClearTimeoutTxStorage,
	route Route,
) RetryTimeoutPacket {
	return RetryTimeoutPacket{txSubmitter: txSubmitter, storage: storage, route: route}
}

func (p RetryTimeoutPacket) Process(ctx context.Context, tr *Transfer) (*Transfer, error) {
	if tr.TimeoutTxHash == nil || tr.TimeoutTxTime == nil {
		return nil, errors.New("transfer has no timeout tx details, violates ShouldProcess")
	}

	retry, err := p.txSubmitter.ShouldRetry(ctx, *tr.TimeoutTxHash, *tr.TimeoutTxTime)
	if err != nil {
		return nil, errors.Wrapf(err, "checking if timeout tx %s should be retried", *tr.TimeoutTxHash)
	}

	if !retry {
		return tr, nil
	}

	if err := p.storage.ClearPacketTimeoutTx(ctx, tr.Key()); err != nil {
		return nil, errors.Wrapf(err, "clearing timeout tx %s", *tr.TimeoutTxHash)
	}

	return nil, ErrRetryingTimeoutPacket
}

func (p RetryTimeoutPacket) Cancel(tr *Transfer, err error) {
	switch {
	case errors.Is(err, ErrRetryingTimeoutPacket):
		tr.GetLogger().Warn("Retrying relay tx", "kind", "timeout")
	case errors.Is(err, v2.ErrTxNotFound):
		tr.GetLogger().Debug("Relay tx not yet found on chain", "kind", "timeout")
	default:
		tr.GetLogger().Error("Checking relay tx retry", "kind", "timeout", "error", err)
	}
}

func (p RetryTimeoutPacket) ShouldProcess(tr *Transfer) bool {
	return tr.TimeoutTxHash != nil && tr.TimeoutTxTime != nil
}

func (p RetryTimeoutPacket) Status() store.RelayStatus {
	return store.RelayStatusDeliverTimeoutPacket
}
