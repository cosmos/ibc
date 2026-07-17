package processors

import (
	"context"
	"time"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/relay/transfer"
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

func (p CheckSendFinality) Process(ctx context.Context, tr *transfer.Transfer) (*transfer.Transfer, error) {
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
		return nil, transfer.ErrSendNotFinalized
	}

	if tr.SourceTxFinalizedTime == nil {
		now := time.Now()
		tr.SourceTxFinalizedTime = &now
	}

	return tr, nil
}

func (p CheckSendFinality) Cancel(tr *transfer.Transfer, err error) {
	if errors.Is(err, transfer.ErrSendNotFinalized) {
		if time.Since(tr.SourceTxTime) > nodeLagWarningAfter {
			tr.GetLogger().Warn("Send tx not finalized after 30 minutes, is the node lagging?", "error", err)
		}

		return
	}

	tr.GetLogger().Error("Checking send tx finality", "error", err)
}

func (p CheckSendFinality) ShouldProcess(tr *transfer.Transfer) bool {
	return tr.RecvTxHash == nil
}

func (p CheckSendFinality) Status() store.RelayStatus {
	return store.RelayStatusAwaitingSendFinality
}
