package processors

import (
	"context"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/relayer/transfer"
	"github.com/cosmos/ibc/link/internal/store"
)

// StatusStorage persists packet status transitions.
type StatusStorage interface {
	UpdatePacketStatus(ctx context.Context, key store.PacketKey, status store.RelayStatus) error
}

// StateFinisher assigns terminal statuses to completed transfers. It is not a
// Processor: it has no status of its own to persist mid-flight.
type StateFinisher struct {
	storage StatusStorage
}

func NewStateFinisher(storage StatusStorage) StateFinisher {
	return StateFinisher{storage: storage}
}

func (p StateFinisher) Process(ctx context.Context, tr *transfer.Transfer) (*transfer.Transfer, error) {
	if tr.Error() != "" {
		return tr, nil
	}

	if !tr.IsComplete() {
		tr.GetLogger().Warn("State finisher received a tr that is neither errored nor complete")

		return tr, nil
	}

	switch {
	case tr.TimeoutTxHash != nil:
		tr.Status = store.RelayStatusCompleteWithTimeout
	case tr.AckTxHash != nil:
		tr.Status = store.RelayStatusCompleteWithAck
	default:
		tr.GetLogger().
			Warn("This is a bug! Completed tr has neither a timeout nor an ack tx, not finishing")

		return tr, nil
	}

	if err := p.storage.UpdatePacketStatus(ctx, tr.Key(), tr.Status); err != nil {
		tr.GetLogger().Error("Updating tr to terminal status", "status", tr.Status, "error", err)
		tr.ProcessingError = errors.Wrapf(err, "updating tr status to %s", tr.Status)

		return tr, nil
	}

	tr.GetLogger().Info("Transfer complete", "status", tr.Status)

	return tr, nil
}

func (p StateFinisher) Cancel(tr *transfer.Transfer, err error) {
	tr.GetLogger().Error("Finishing tr state", "error", err)
}
