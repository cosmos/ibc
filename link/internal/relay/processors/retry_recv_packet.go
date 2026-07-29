package processors

import (
	"context"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/store"
	"github.com/cosmos/ibc/link/internal/txsubmitter"

	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

// ClearRecvTxStorage clears a recorded relay tx so it is resubmitted.
type ClearRecvTxStorage interface {
	ClearPacketRecvTx(ctx context.Context, key store.PacketKey) error
}

// RetryRecvPacket clears a stuck or failed recv tx so the packet is
// redelivered on the next run.
type RetryRecvPacket struct {
	txSubmitter txsubmitter.TxSubmitter
	storage     ClearRecvTxStorage
	route       Route
}

func NewRetryRecvPacket(txSubmitter txsubmitter.TxSubmitter, storage ClearRecvTxStorage, route Route) RetryRecvPacket {
	return RetryRecvPacket{txSubmitter: txSubmitter, storage: storage, route: route}
}

func (p RetryRecvPacket) Process(ctx context.Context, tr *Transfer) (*Transfer, error) {
	if tr.RecvTxHash == nil || tr.RecvTxTime == nil {
		return nil, errors.New("transfer has no recv tx details, violates ShouldProcess")
	}

	retry, err := p.txSubmitter.ShouldRetry(ctx, *tr.RecvTxHash, *tr.RecvTxTime)
	if err != nil {
		return nil, errors.Wrapf(err, "checking if recv tx %s should be retried", *tr.RecvTxHash)
	}

	if !retry {
		return tr, nil
	}

	if err := p.storage.ClearPacketRecvTx(ctx, tr.Key()); err != nil {
		return nil, errors.Wrapf(err, "clearing recv tx %s", *tr.RecvTxHash)
	}

	// error so the transfer stops processing this run; it is picked up
	// without the recv tx and redelivered on the next run
	return nil, ErrRetryingRecvPacket
}

func (p RetryRecvPacket) Cancel(tr *Transfer, err error) {
	switch {
	case errors.Is(err, ErrRetryingRecvPacket):
		tr.GetLogger().Warn("Retrying relay tx", "kind", "recv")
	case errors.Is(err, v2.ErrTxNotFound):
		tr.GetLogger().Debug("Relay tx not yet found on chain", "kind", "recv")
	default:
		tr.GetLogger().Error("Checking relay tx retry", "kind", "recv", "err", err)
	}
}

func (p RetryRecvPacket) ShouldProcess(tr *Transfer) bool {
	return tr.RecvTxHash != nil && tr.RecvTxTime != nil && tr.WriteAckTxHash == nil
}

func (p RetryRecvPacket) Status() store.RelayStatus {
	return store.RelayStatusDeliverRecvPacket
}
