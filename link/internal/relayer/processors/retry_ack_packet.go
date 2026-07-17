//nolint:dupl // the retry directions are structurally parallel by design
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

// RetryAckExpiry is how long a submitted relay tx may sit unconfirmed before it is
// cleared and redelivered.
const RetryAckExpiry = 2 * time.Minute

// ClearAckTxStorage clears a recorded relay tx so it is resubmitted.
type ClearAckTxStorage interface {
	ClearPacketAckTx(ctx context.Context, key store.PacketKey) error
}

// RetryAckPacket clears a stuck or failed ack tx so the ack is redelivered on
// the next run.
type RetryAckPacket struct {
	txManager txmgr.TxManager
	storage   ClearAckTxStorage
	route     transfer.Route
}

func NewRetryAckPacket(txManager txmgr.TxManager, storage ClearAckTxStorage, route transfer.Route) RetryAckPacket {
	return RetryAckPacket{txManager: txManager, storage: storage, route: route}
}

func (p RetryAckPacket) Process(ctx context.Context, tr *transfer.Transfer) (*transfer.Transfer, error) {
	if tr.AckTxHash == nil || tr.AckTxTime == nil {
		return nil, errors.New("transfer has no ack tx details, violates ShouldProcess")
	}

	retry, err := p.txManager.ShouldRetry(
		ctx,
		*tr.AckTxHash,
		RetryAckExpiry,
		*tr.AckTxTime,
	)
	if err != nil {
		return nil, errors.Wrapf(err, "checking if ack tx %s should be retried", *tr.AckTxHash)
	}

	if !retry {
		return tr, nil
	}

	if err := p.storage.ClearPacketAckTx(ctx, tr.Key()); err != nil {
		return nil, errors.Wrapf(err, "clearing ack tx %s", *tr.AckTxHash)
	}

	return nil, transfer.ErrRetryingAckPacket
}

func (p RetryAckPacket) Cancel(tr *transfer.Transfer, err error) {
	switch {
	case errors.Is(err, transfer.ErrRetryingAckPacket):
		tr.GetLogger().Warn("Retrying relay tx", "kind", "ack")
	case errors.Is(err, v2.ErrTxNotFound):
		tr.GetLogger().Debug("Relay tx not yet found on chain", "kind", "ack")
	default:
		tr.GetLogger().Error("Checking relay tx retry", "kind", "ack", "error", err)
	}
}

func (p RetryAckPacket) ShouldProcess(tr *transfer.Transfer) bool {
	return tr.AckTxHash != nil && tr.AckTxTime != nil
}

func (p RetryAckPacket) Status() store.RelayStatus {
	return store.RelayStatusDeliverAckPacket
}
