package processors

import (
	"context"
	"time"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/relayer/transfer"
	"github.com/cosmos/ibc/link/internal/store"
	"github.com/cosmos/ibc/link/internal/txmgr"

	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

// RetryRecvExpiry is how long a submitted relay tx may sit unconfirmed before it is
// cleared and redelivered.
const RetryRecvExpiry = 2 * time.Minute

// ClearRecvTxStorage clears a recorded relay tx so it is resubmitted.
type ClearRecvTxStorage interface {
	ClearPacketRecvTx(ctx context.Context, key store.PacketKey) error
}

// RetryRecvPacket clears a stuck or failed recv tx so the packet is
// redelivered on the next run.
type RetryRecvPacket struct {
	txManager txmgr.TxManager
	storage   ClearRecvTxStorage
	route     transfer.Route
}

func NewRetryRecvPacket(txManager txmgr.TxManager, storage ClearRecvTxStorage, route transfer.Route) RetryRecvPacket {
	return RetryRecvPacket{txManager: txManager, storage: storage, route: route}
}

func (p RetryRecvPacket) Process(ctx context.Context, tr *transfer.Transfer) (*transfer.Transfer, error) {
	if tr.RecvTxHash == nil || tr.RecvTxTime == nil {
		return nil, errors.New("transfer has no recv tx details, violates ShouldProcess")
	}

	retry, err := p.txManager.ShouldRetry(
		ctx,
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
	switch {
	case errors.Is(err, transfer.ErrRetryingRecvPacket):
		tr.GetLogger().Warn("Retrying relay tx", "kind", "recv")
	case errors.Is(err, v2.ErrTxNotFound):
		tr.GetLogger().Debug("Relay tx not yet found on chain", "kind", "recv")
	default:
		tr.GetLogger().Error("Checking relay tx retry", "kind", "recv", "error", err)
	}
}

func (p RetryRecvPacket) ShouldProcess(tr *transfer.Transfer) bool {
	return tr.RecvTxHash != nil && tr.RecvTxTime != nil && tr.WriteAckTxHash == nil
}

func (p RetryRecvPacket) Status() store.RelayStatus {
	return store.RelayStatusDeliverRecvPacket
}
