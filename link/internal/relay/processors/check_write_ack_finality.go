// SPDX-License-Identifier: Apache-2.0

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

// CheckWriteAckFinality gates ack relaying on the write ack tx's height
// being at or before the height the source client's proof generator can
// currently prove.
type CheckWriteAckFinality struct {
	destinationChainClient chains.Client
	proofGen               proofgen.ProofGenerator
}

func NewCheckWriteAckFinality(
	chainClients ChainClients,
	proofGenerators ProofGenerators,
	route Route,
) (CheckWriteAckFinality, error) {
	destinationChainClient, ok := chainClients.Get(route.DestinationChainID)
	if !ok {
		return CheckWriteAckFinality{}, errors.Errorf(
			"no configured chain client for destination chain %s", route.DestinationChainID,
		)
	}

	proofGen, ok := proofGenerators.Get(route.SourceChainID, route.SourceClientID)
	if !ok {
		return CheckWriteAckFinality{}, errors.Errorf(
			"no proof generator configured for client %q on chain %q", route.SourceClientID, route.SourceChainID,
		)
	}

	return CheckWriteAckFinality{destinationChainClient: destinationChainClient, proofGen: proofGen}, nil
}

func (p CheckWriteAckFinality) Process(ctx context.Context, tr *Transfer) (*Transfer, error) {
	if tr.WriteAckTxHash == nil {
		return nil, errors.New("transfer has no write ack tx hash, violates ShouldProcess")
	}

	proofHeight, _, err := p.proofGen.LatestProvableHeight(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "resolving latest provable height")
	}

	txID, err := hex.DecodeString(strings.TrimPrefix(*tr.WriteAckTxHash, "0x"))
	if err != nil {
		return nil, errors.Wrapf(err, "decoding tx hash %q", *tr.WriteAckTxHash)
	}

	txHeight, err := p.destinationChainClient.TxHeight(ctx, txID)
	if err != nil {
		return nil, errors.Wrapf(err, "reading tx height for tx %s", *tr.WriteAckTxHash)
	}

	if txHeight > proofHeight {
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
			tr.GetLogger().Warn("Write ack tx not finalized after 30 minutes, is the node lagging?", "err", err)
		}

		return
	}

	tr.GetLogger().Error("Checking write ack finality", "err", err)
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
