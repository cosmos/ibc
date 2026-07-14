package pipeline

import (
	"context"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/store"
)

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

func (p StateFinisher) Process(ctx context.Context, transfer *Transfer) (*Transfer, error) {
	if transfer.Error() != "" {
		return transfer, nil
	}

	if !transfer.IsComplete(p.relaySuccessAcks, p.relayErrorAcks) {
		transfer.GetLogger().Warn("State finisher received a transfer that is neither errored nor complete")

		return transfer, nil
	}

	switch {
	case transfer.TimeoutTxHash != nil:
		transfer.Status = store.RelayStatusCompleteWithTimeout
	case transfer.WriteAckStatus == nil:
		transfer.GetLogger().
			Warn("This is a bug! Completed non-timeout transfer has no write ack status, not finishing")

		return transfer, nil
	case transfer.AckTxHash != nil:
		transfer.Status = store.RelayStatusCompleteWithAck
	case isErrorAck(*transfer.WriteAckStatus):
		transfer.Status = store.RelayStatusCompleteWithWriteAckError
	default:
		transfer.Status = store.RelayStatusCompleteWithWriteAckSuccess
	}

	if err := p.storage.UpdatePacketStatus(ctx, transfer.Key(), transfer.Status); err != nil {
		transfer.GetLogger().Error("Updating transfer to terminal status", "status", transfer.Status, "error", err)
		transfer.ProcessingError = errors.Wrapf(err, "updating transfer status to %s", transfer.Status)

		return transfer, nil
	}

	transfer.GetLogger().Info("Transfer complete", "status", transfer.Status)

	return transfer, nil
}

func (p StateFinisher) Cancel(transfer *Transfer, err error) {
	transfer.GetLogger().Error("Finishing transfer state", "error", err)
}
