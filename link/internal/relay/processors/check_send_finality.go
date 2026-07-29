package processors

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

func (p CheckSendFinality) Process(ctx context.Context, tr *Transfer) (*Transfer, error) {
	client, ok := p.chains.Get(tr.SourceChainID)
	if !ok {
		return nil, errors.Errorf("no configured chain client for source chain %s", tr.SourceChainID)
	}

	finalized, err := client.IsTxFinalized(ctx, tr.SourceTxHash, p.finalityOffset)
	if err != nil {
		return nil, errors.Wrapf(
			err,
			"checking finality of tx %s on chain %s",
			tr.SourceTxHash,
			tr.SourceChainID,
		)
	}

	if !finalized {
		return nil, ErrSendNotFinalized
	}

	if tr.SourceTxFinalizedTime == nil {
		now := time.Now()
		tr.SourceTxFinalizedTime = &now
	}

	return tr, nil
}

func (p CheckSendFinality) Cancel(tr *Transfer, err error) {
	if errors.Is(err, ErrSendNotFinalized) {
		if time.Since(tr.SourceTxTime) > nodeLagWarningAfter {
			tr.GetLogger().Warn("Send tx not finalized after 30 minutes, is the node lagging?", "err", err)
		}

		return
	}

	tr.GetLogger().Error("Checking send tx finality", "err", err)
}

func (p CheckSendFinality) ShouldProcess(tr *Transfer) bool {
	return tr.RecvTxHash == nil
}

func (p CheckSendFinality) Status() store.RelayStatus {
	return store.RelayStatusAwaitingSendFinality
}
