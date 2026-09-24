// SPDX-License-Identifier: Apache-2.0

package deploy

import (
	"context"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besumsgs"
	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"
)

// BesuQBFTParams are the constructor inputs for a Besu QBFT client. Periods
// are in seconds.
type BesuQBFTParams struct {
	IBCRouter             common.Address
	InitialHeight         uint64
	InitialConsensusState besumsgs.IBesuLightClientMsgsConsensusState
	TrustingPeriod        uint64
	MaxClockDrift         uint64
}

// BesuQBFTSource is implemented by targets whose chain runs Besu QBFT and can
// serve the consensus state a client tracking it starts trusting.
type BesuQBFTSource interface {
	BesuQBFTConsensusState(ctx context.Context, height uint64) (besumsgs.IBesuLightClientMsgsConsensusState, error)
}

// BesuQBFTBootstrap describes a new Besu QBFT client on Host proving
// IBCRouter on Counterparty. Periods are in seconds.
type BesuQBFTBootstrap struct {
	Host           Target
	Counterparty   Target
	IBCRouter      common.Address
	Height         uint64 // 0: the counterparty head
	TrustingPeriod uint64
	MaxClockDrift  uint64
}

// Params reads the counterparty consensus state the client starts trusting.
func (b BesuQBFTBootstrap) Params(ctx context.Context) (BesuQBFTParams, error) {
	source, ok := b.Counterparty.(BesuQBFTSource)
	if !ok {
		return BesuQBFTParams{}, errors.New("counterparty cannot serve a besu-qbft trusted state")
	}
	height := b.Height
	if height == 0 {
		head, _, err := b.Counterparty.Head(ctx)
		if err != nil {
			return BesuQBFTParams{}, errors.Wrap(err, "fetch counterparty head for initial trusted state")
		}
		if head == 0 {
			return BesuQBFTParams{}, errors.New("counterparty has no block past genesis to trust yet")
		}
		height = head
	}
	state, err := source.BesuQBFTConsensusState(ctx, height)
	if err != nil {
		return BesuQBFTParams{}, errors.Wrap(err, "read counterparty trusted state")
	}
	// the contract measures expiry against the host's block time, so a trusted
	// state that is already expired there can never be updated
	_, hostTime, err := b.Host.Head(ctx)
	if err != nil {
		return BesuQBFTParams{}, errors.Wrap(err, "fetch head")
	}
	if hostTime >= state.Timestamp && hostTime-state.Timestamp >= b.TrustingPeriod {
		return BesuQBFTParams{}, errors.Errorf(
			"counterparty trusted state at height %d (timestamp %d) is already older than the "+
				"trusting period (%ds) at this chain's head (timestamp %d): pick a newer height or a longer trusting period",
			height, state.Timestamp, b.TrustingPeriod, hostTime,
		)
	}
	return BesuQBFTParams{
		IBCRouter:             b.IBCRouter,
		InitialHeight:         height,
		InitialConsensusState: state,
		TrustingPeriod:        b.TrustingPeriod,
		MaxClockDrift:         b.MaxClockDrift,
	}, nil
}
