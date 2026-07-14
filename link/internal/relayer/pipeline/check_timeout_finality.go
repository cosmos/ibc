package pipeline

import (
	"context"
	"time"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/store"
)

// CheckTimeoutFinality gates timing out a packet on the destination chain
// having finalized a block past the timeout timestamp.
type CheckTimeoutFinality struct {
	chains         ChainClients
	finalityOffset *uint64
}

func NewCheckTimeoutFinality(chainClients ChainClients, finalityOffset *uint64) CheckTimeoutFinality {
	return CheckTimeoutFinality{chains: chainClients, finalityOffset: finalityOffset}
}

func (p CheckTimeoutFinality) Process(ctx context.Context, transfer *Transfer) (*Transfer, error) {
	client, ok := p.chains.Get(transfer.DestinationChainID)
	if !ok {
		return nil, errors.Errorf("no configured chain client for destination chain %s", transfer.DestinationChainID)
	}

	finalized, err := client.IsTimestampFinalized(ctx, transfer.PacketTimeoutTimestamp, p.finalityOffset)
	if err != nil {
		return nil, errors.Wrapf(err, "checking timeout finality on chain %s", transfer.DestinationChainID)
	}

	if !finalized {
		return nil, ErrTimeoutNotFinalized
	}

	return transfer, nil
}

func (p CheckTimeoutFinality) Cancel(transfer *Transfer, err error) {
	if errors.Is(err, ErrTimeoutNotFinalized) {
		if time.Since(transfer.PacketTimeoutTimestamp) > nodeLagWarningAfter {
			transfer.GetLogger().
				Warn("Timeout timestamp not finalized 30 minutes past timeout, is the node lagging?", "error", err)
		}

		return
	}

	transfer.GetLogger().Error("Checking timeout finality", "error", err)
}

func (p CheckTimeoutFinality) ShouldProcess(transfer *Transfer) bool {
	shouldBeTimedOut := transfer.IsTimedOut() && transfer.RecvTxHash == nil && transfer.AckTxHash == nil

	return shouldBeTimedOut && transfer.TimeoutTxHash == nil
}

func (p CheckTimeoutFinality) Status() store.RelayStatus {
	return store.RelayStatusAwaitingTimeoutFinality
}
