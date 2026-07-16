package processors

import (
	"context"
	"time"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/relayer/transfer"
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

func (p CheckTimeoutFinality) Process(ctx context.Context, tr *transfer.Transfer) (*transfer.Transfer, error) {
	client, ok := p.chains.Get(tr.DestinationChainID)
	if !ok {
		return nil, errors.Errorf("no configured chain client for destination chain %s", tr.DestinationChainID)
	}

	finalized, err := client.IsTimestampFinalized(ctx, tr.PacketTimeoutTimestamp, p.finalityOffset)
	if err != nil {
		return nil, errors.Wrapf(err, "checking timeout finality on chain %s", tr.DestinationChainID)
	}

	if !finalized {
		return nil, transfer.ErrTimeoutNotFinalized
	}

	return tr, nil
}

func (p CheckTimeoutFinality) Cancel(tr *transfer.Transfer, err error) {
	if errors.Is(err, transfer.ErrTimeoutNotFinalized) {
		if time.Since(tr.PacketTimeoutTimestamp) > nodeLagWarningAfter {
			tr.GetLogger().
				Warn("Timeout timestamp not finalized 30 minutes past timeout, is the node lagging?", "error", err)
		}

		return
	}

	tr.GetLogger().Error("Checking timeout finality", "error", err)
}

func (p CheckTimeoutFinality) ShouldProcess(tr *transfer.Transfer) bool {
	shouldBeTimedOut := tr.IsTimedOut() && tr.RecvTxHash == nil && tr.AckTxHash == nil

	return shouldBeTimedOut && tr.TimeoutTxHash == nil
}

func (p CheckTimeoutFinality) Status() store.RelayStatus {
	return store.RelayStatusAwaitingTimeoutFinality
}
