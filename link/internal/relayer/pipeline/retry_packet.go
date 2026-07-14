package pipeline

import (
	"context"
	"time"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/store"

	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

// Relay tx retry expiries
const (
	RetryRecvExpiry    = 2 * time.Minute
	RetryAckExpiry     = 2 * time.Minute
	RetryTimeoutExpiry = 2 * time.Minute
)

// ClearTxStorage clears recorded relay txs so they are resubmitted.
type ClearTxStorage interface {
	ClearPacketRecvTx(ctx context.Context, key store.PacketKey) error
	ClearPacketAckTx(ctx context.Context, key store.PacketKey) error
	ClearPacketTimeoutTx(ctx context.Context, key store.PacketKey) error
}

func cancelRetry(transfer *Transfer, err error, retrySentinel error, kind string) {
	switch {
	case errors.Is(err, retrySentinel):
		transfer.GetLogger().Warn("Retrying relay tx", "kind", kind)
	case errors.Is(err, v2.ErrTxNotFound):
		transfer.GetLogger().Debug("Relay tx not yet found on chain", "kind", kind)
	default:
		transfer.GetLogger().Error("Checking relay tx retry", "kind", kind, "error", err)
	}
}
