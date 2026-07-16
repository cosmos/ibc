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
	storage          StatusStorage
	relaySuccessAcks bool
	relayErrorAcks   bool
}

func NewStateFinisher(storage StatusStorage, relaySuccessAcks, relayErrorAcks bool) StateFinisher {
	return StateFinisher{storage: storage, relaySuccessAcks: relaySuccessAcks, relayErrorAcks: relayErrorAcks}
}

func (p StateFinisher) Process(ctx context.Context, tr *transfer.Transfer) (*transfer.Transfer, error) {
	if tr.Error() != "" {
		return tr, nil
	}

	if !tr.IsComplete(p.relaySuccessAcks, p.relayErrorAcks) {
		tr.GetLogger().Warn("State finisher received a tr that is neither errored nor complete")

		return tr, nil
	}

	switch {
	case tr.TimeoutTxHash != nil:
		tr.Status = store.RelayStatusCompleteWithTimeout
	case tr.WriteAckStatus == nil:
		tr.GetLogger().
			Warn("This is a bug! Completed non-timeout tr has no write ack status, not finishing")

		return tr, nil
	case tr.AckTxHash != nil:
		tr.Status = store.RelayStatusCompleteWithAck
	case transfer.IsErrorAck(*tr.WriteAckStatus):
		tr.Status = store.RelayStatusCompleteWithWriteAckError
	default:
		tr.Status = store.RelayStatusCompleteWithWriteAckSuccess
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
