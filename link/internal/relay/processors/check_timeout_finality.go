// SPDX-License-Identifier: Apache-2.0

package processors

import (
	"context"
	"time"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/relay/proofgen"
	"github.com/cosmos/ibc/link/internal/store"
)

// CheckTimeoutFinality gates timing out a packet on the source client's
// proof generator currently being able to prove a destination-chain
// timestamp past the timeout.
type CheckTimeoutFinality struct {
	proofGen proofgen.Prover
}

func NewCheckTimeoutFinality(proofGenerators ProofGenerators, route Route) (CheckTimeoutFinality, error) {
	proofGen, ok := proofGenerators.Get(route.SourceChainID, route.SourceClientID)
	if !ok {
		return CheckTimeoutFinality{}, errors.Errorf(
			"no proof generator configured for client %q on chain %q", route.SourceClientID, route.SourceChainID,
		)
	}

	return CheckTimeoutFinality{proofGen: proofGen}, nil
}

func (p CheckTimeoutFinality) Process(ctx context.Context, tr *Transfer) (*Transfer, error) {
	_, timestamp, err := p.proofGen.LatestProvableHeight(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "resolving latest provable height")
	}

	if tr.PacketTimeoutTimestamp.After(timestamp) {
		return nil, ErrTimeoutNotFinalized
	}

	return tr, nil
}

func (p CheckTimeoutFinality) Cancel(tr *Transfer, err error) {
	if errors.Is(err, ErrTimeoutNotFinalized) {
		if time.Since(tr.PacketTimeoutTimestamp) > nodeLagWarningAfter {
			tr.GetLogger().
				Warn("Timeout timestamp not finalized 30 minutes past timeout, is the node lagging?", "err", err)
		}

		return
	}

	tr.GetLogger().Error("Checking timeout finality", "err", err)
}

func (p CheckTimeoutFinality) ShouldProcess(tr *Transfer) bool {
	shouldBeTimedOut := tr.IsTimedOut() && tr.RecvTxHash == nil && tr.AckTxHash == nil

	return shouldBeTimedOut && tr.TimeoutTxHash == nil
}

func (p CheckTimeoutFinality) Status() store.RelayStatus {
	return store.RelayStatusAwaitingTimeoutFinality
}
