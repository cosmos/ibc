// SPDX-License-Identifier: Apache-2.0

package processors

import (
	"context"
	"encoding/hex"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/cli/internal/chains"
	"github.com/cosmos/ibc/cli/internal/relay/prover"
	"github.com/cosmos/ibc/cli/internal/store"
)

// CheckSendFinality gates relaying on the send tx's height being at or
// before the height the destination client's prover can currently
// prove.
type CheckSendFinality struct {
	sourceChainClient chains.Client
	prover            prover.Prover
}

func NewCheckSendFinality(route RouteDeps) CheckSendFinality {
	return CheckSendFinality{sourceChainClient: route.Source.Chain, prover: route.Destination.Prover}
}

func (p CheckSendFinality) Process(ctx context.Context, tr *Transfer) (*Transfer, error) {
	proofHeight, _, err := p.prover.LatestProvableHeight(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "resolving latest provable height")
	}

	txID, err := hex.DecodeString(strings.TrimPrefix(tr.SourceTxHash, "0x"))
	if err != nil {
		return nil, errors.Wrapf(err, "decoding tx hash %q", tr.SourceTxHash)
	}

	txHeight, err := p.sourceChainClient.TxHeight(ctx, txID)
	if err != nil {
		return nil, errors.Wrapf(err, "reading tx height for tx %s", tr.SourceTxHash)
	}

	if txHeight > proofHeight {
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
