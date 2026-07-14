package pipeline

import (
	"context"
	"time"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/store"
)

// CheckWriteAckFinality gates ack relaying on the write ack tx being finalized
// on the destination chain.
type CheckWriteAckFinality struct {
	chains           ChainClients
	relaySuccessAcks bool
	relayErrorAcks   bool
	finalityOffset   *uint64
}

func NewCheckWriteAckFinality(
	chainClients ChainClients,
	relaySuccessAcks, relayErrorAcks bool,
	finalityOffset *uint64,
) CheckWriteAckFinality {
	return CheckWriteAckFinality{
		chains:           chainClients,
		relaySuccessAcks: relaySuccessAcks,
		relayErrorAcks:   relayErrorAcks,
		finalityOffset:   finalityOffset,
	}
}

func (p CheckWriteAckFinality) Process(ctx context.Context, transfer *Transfer) (*Transfer, error) {
	client, ok := p.chains.Get(transfer.DestinationChainID)
	if !ok {
		return nil, errors.Errorf("no configured chain client for destination chain %s", transfer.DestinationChainID)
	}

	if transfer.WriteAckTxHash == nil {
		return nil, errors.New("transfer has no write ack tx hash, violates ShouldProcess")
	}

	finalized, err := client.IsTxFinalized(ctx, *transfer.WriteAckTxHash, p.finalityOffset)
	if err != nil {
		return nil, errors.Wrapf(err, "checking finality of write ack tx %s", *transfer.WriteAckTxHash)
	}

	if !finalized {
		return nil, ErrWriteAckNotFinalized
	}

	if transfer.WriteAckTxFinalizedTime == nil {
		now := time.Now()
		transfer.WriteAckTxFinalizedTime = &now
	}

	return transfer, nil
}

func (p CheckWriteAckFinality) Cancel(transfer *Transfer, err error) {
	if errors.Is(err, ErrWriteAckNotFinalized) {
		if transfer.WriteAckTxTime != nil && time.Since(*transfer.WriteAckTxTime) > nodeLagWarningAfter {
			transfer.GetLogger().Warn("Write ack tx not finalized after 30 minutes, is the node lagging?", "error", err)
		}

		return
	}

	transfer.GetLogger().Error("Checking write ack finality", "error", err)
}

func (p CheckWriteAckFinality) ShouldProcess(transfer *Transfer) bool {
	if transfer.WriteAckTxHash == nil {
		return false
	}

	if transfer.WriteAckStatus == nil {
		transfer.GetLogger().Warn("This is a bug! Transfer has a write ack tx hash but no write ack status")

		return false
	}

	return (isErrorAck(*transfer.WriteAckStatus) && p.relayErrorAcks) ||
		(isSuccessAck(*transfer.WriteAckStatus) && p.relaySuccessAcks)
}

func (p CheckWriteAckFinality) Status() store.RelayStatus {
	return store.RelayStatusAwaitingWriteAckFinality
}
