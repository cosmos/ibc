package processors

import (
	"context"
	"time"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/store"
)

// CheckWriteAckFinality gates ack relaying on the write ack tx being finalized
// on the destination chain.
type CheckWriteAckFinality struct {
	chains         ChainClients
	finalityOffset *uint64
}

func NewCheckWriteAckFinality(chainClients ChainClients, finalityOffset *uint64) CheckWriteAckFinality {
	return CheckWriteAckFinality{chains: chainClients, finalityOffset: finalityOffset}
}

func (p CheckWriteAckFinality) Process(ctx context.Context, tr *Transfer) (*Transfer, error) {
	client, ok := p.chains.Get(tr.DestinationChainID)
	if !ok {
		return nil, errors.Errorf("no configured chain client for destination chain %s", tr.DestinationChainID)
	}

	if tr.WriteAckTxHash == nil {
		return nil, errors.New("transfer has no write ack tx hash, violates ShouldProcess")
	}

	finalized, err := client.IsTxFinalized(ctx, *tr.WriteAckTxHash, p.finalityOffset)
	if err != nil {
		return nil, errors.Wrapf(err, "checking finality of write ack tx %s", *tr.WriteAckTxHash)
	}

	if !finalized {
		return nil, ErrWriteAckNotFinalized
	}

	if tr.WriteAckTxFinalizedTime == nil {
		now := time.Now()
		tr.WriteAckTxFinalizedTime = &now
	}

	return tr, nil
}

func (p CheckWriteAckFinality) Cancel(tr *Transfer, err error) {
	if errors.Is(err, ErrWriteAckNotFinalized) {
		if tr.WriteAckTxTime != nil && time.Since(*tr.WriteAckTxTime) > nodeLagWarningAfter {
			tr.GetLogger().Warn("Write ack tx not finalized after 30 minutes, is the node lagging?", "error", err)
		}

		return
	}

	tr.GetLogger().Error("Checking write ack finality", "error", err)
}

func (p CheckWriteAckFinality) ShouldProcess(tr *Transfer) bool {
	if tr.WriteAckTxHash == nil {
		return false
	}

	return tr.AckTxHash == nil && tr.TimeoutTxHash == nil
}

func (p CheckWriteAckFinality) Status() store.RelayStatus {
	return store.RelayStatusAwaitingWriteAckFinality
}
