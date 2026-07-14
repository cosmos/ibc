package pipeline

import (
	"context"
	"time"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/store"
)

// CheckSendFinality gates relaying on the send tx being finalized on the
// source chain.
type CheckSendFinality struct {
	chains         ChainClients
	finalityOffset *uint64
}

func NewCheckSendFinality(chainClients ChainClients, finalityOffset *uint64) CheckSendFinality {
	return CheckSendFinality{chains: chainClients, finalityOffset: finalityOffset}
}

func (p CheckSendFinality) Process(ctx context.Context, transfer *Transfer) (*Transfer, error) {
	client, ok := p.chains.Get(transfer.SourceChainID)
	if !ok {
		return nil, errors.Errorf("no configured chain client for source chain %s", transfer.SourceChainID)
	}

	finalized, err := client.IsTxFinalized(ctx, transfer.SourceTxHash, p.finalityOffset)
	if err != nil {
		return nil, errors.Wrapf(
			err,
			"checking finality of tx %s on chain %s",
			transfer.SourceTxHash,
			transfer.SourceChainID,
		)
	}

	if !finalized {
		return nil, ErrSendNotFinalized
	}

	if transfer.SourceTxFinalizedTime == nil {
		now := time.Now()
		transfer.SourceTxFinalizedTime = &now
	}

	return transfer, nil
}

func (p CheckSendFinality) Cancel(transfer *Transfer, err error) {
	if errors.Is(err, ErrSendNotFinalized) {
		if time.Since(transfer.SourceTxTime) > nodeLagWarningAfter {
			transfer.GetLogger().Warn("Send tx not finalized after 30 minutes, is the node lagging?", "error", err)
		}

		return
	}

	transfer.GetLogger().Error("Checking send tx finality", "error", err)
}

func (p CheckSendFinality) ShouldProcess(transfer *Transfer) bool {
	return transfer.RecvTxHash == nil
}

func (p CheckSendFinality) Status() store.RelayStatus {
	return store.RelayStatusAwaitingSendFinality
}
