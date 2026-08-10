package processors

import (
	"context"
	"encoding/hex"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/relay/proofgen"
	"github.com/cosmos/ibc/link/internal/store"
)

// CheckSendFinality gates relaying on the send tx's height being at or
// before the height the destination client's proof generator can currently
// prove.
type CheckSendFinality struct {
	sourceChainClient chains.Client
	proofGen          proofgen.ProofGenerator
}

func NewCheckSendFinality(
	chainClients ChainClients,
	proofGenerators ProofGenerators,
	route Route,
) (CheckSendFinality, error) {
	sourceChainClient, ok := chainClients.Get(route.SourceChainID)
	if !ok {
		return CheckSendFinality{}, errors.Errorf("no configured chain client for source chain %s", route.SourceChainID)
	}

	proofGen, ok := proofGenerators.Get(route.DestinationChainID, route.DestinationClientID)
	if !ok {
		return CheckSendFinality{}, errors.Errorf(
			"no proof generator configured for client %q on chain %q",
			route.DestinationClientID, route.DestinationChainID,
		)
	}

	return CheckSendFinality{sourceChainClient: sourceChainClient, proofGen: proofGen}, nil
}

func (p CheckSendFinality) Process(ctx context.Context, tr *Transfer) (*Transfer, error) {
	proofHeight, _, err := p.proofGen.LatestProvableHeight(ctx)
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
