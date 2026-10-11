// SPDX-License-Identifier: Apache-2.0

package processors

import (
	"context"
	"encoding/hex"
	"strings"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/cli/internal/chains"
	"github.com/cosmos/ibc/cli/internal/relay/prover"
	"github.com/cosmos/ibc/cli/internal/store"
)

// CheckSettlementFinality keeps packets dispatchable until their ack or timeout is finalized.
type CheckSettlementFinality struct {
	client chains.Client
	prover prover.Prover
}

func NewCheckSettlementFinality(
	chainClients ChainClients,
	provers Provers,
	route Route,
) (CheckSettlementFinality, error) {
	client, ok := chainClients.Get(route.SourceChainID)
	if !ok {
		return CheckSettlementFinality{}, errors.Errorf("no configured chain %s", route.SourceChainID)
	}
	proof, ok := provers.Get(route.DestinationChainID, route.DestinationClientID)
	if !ok {
		return CheckSettlementFinality{}, errors.Errorf(
			"no prover for client %s on chain %s",
			route.DestinationClientID,
			route.DestinationChainID,
		)
	}
	return CheckSettlementFinality{client: client, prover: proof}, nil
}

func (p CheckSettlementFinality) Process(ctx context.Context, tr *Transfer) (*Transfer, error) {
	hash := tr.AckTxHash
	if tr.TimeoutTxHash != nil {
		hash = tr.TimeoutTxHash
	}
	raw, err := hex.DecodeString(strings.TrimPrefix(*hash, "0x"))
	if err != nil {
		return nil, err
	}
	height, err := p.client.TxHeight(ctx, raw)
	if err != nil {
		return nil, err
	}
	finalized, _, err := p.prover.LatestProvableHeight(ctx)
	if err != nil {
		return nil, err
	}
	if height > finalized {
		return nil, ErrSubmissionPending
	}
	return tr, nil
}

func (p CheckSettlementFinality) ShouldProcess(tr *Transfer) bool {
	return tr.AckTxHash != nil || tr.TimeoutTxHash != nil
}

func (p CheckSettlementFinality) Status() store.RelayStatus {
	return store.RelayStatusCheckAckPacketDelivery
}

func (p CheckSettlementFinality) Cancel(tr *Transfer, err error) {
	if errors.Is(err, ErrSubmissionPending) {
		return
	}
	tr.GetLogger().Error("Checking settlement finality", "err", err)
}
