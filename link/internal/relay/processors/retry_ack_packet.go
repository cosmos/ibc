//nolint:dupl // the retry directions are structurally parallel by design
package processors

import (
	"context"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/store"
	"github.com/cosmos/ibc/link/internal/txsubmitter"

	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

// ClearAckTxStorage clears a recorded relay tx so it is resubmitted.
type ClearAckTxStorage interface {
	ClearPacketAckTx(ctx context.Context, key store.PacketKey) error
}

// RetryAckPacket clears a stuck or failed ack tx so the ack is redelivered on
// the next run.
type RetryAckPacket struct {
	txSubmitter txsubmitter.TxSubmitter
	storage     ClearAckTxStorage
	route       Route
}

func NewRetryAckPacket(txSubmitter txsubmitter.TxSubmitter, storage ClearAckTxStorage, route Route) RetryAckPacket {
	return RetryAckPacket{txSubmitter: txSubmitter, storage: storage, route: route}
}

func (p RetryAckPacket) Process(ctx context.Context, tr *Transfer) (*Transfer, error) {
	if tr.AckTxHash == nil || tr.AckTxTime == nil {
		return nil, errors.New("transfer has no ack tx details, violates ShouldProcess")
	}

	retry, err := p.txSubmitter.ShouldRetry(ctx, *tr.AckTxHash, *tr.AckTxTime)
	if err != nil {
		return nil, errors.Wrapf(err, "checking if ack tx %s should be retried", *tr.AckTxHash)
	}

	if !retry {
		return tr, nil
	}

	if err := p.storage.ClearPacketAckTx(ctx, tr.Key()); err != nil {
		return nil, errors.Wrapf(err, "clearing ack tx %s", *tr.AckTxHash)
	}

	return nil, ErrRetryingAckPacket
}

func (p RetryAckPacket) Cancel(tr *Transfer, err error) {
	switch {
	case errors.Is(err, ErrRetryingAckPacket):
		tr.GetLogger().Warn("Retrying relay tx", "kind", "ack")
	case errors.Is(err, v2.ErrTxNotFound):
		tr.GetLogger().Debug("Relay tx not yet found on chain", "kind", "ack")
	default:
		tr.GetLogger().Error("Checking relay tx retry", "kind", "ack", "error", err)
	}
}

func (p RetryAckPacket) ShouldProcess(tr *Transfer) bool {
	return tr.AckTxHash != nil && tr.AckTxTime != nil
}

func (p RetryAckPacket) Status() store.RelayStatus {
	return store.RelayStatusDeliverAckPacket
}
