// SPDX-License-Identifier: Apache-2.0

package deploy

import (
	"context"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besumsgs"
	"github.com/ethereum/go-ethereum/common"
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

// ClientType implements ClientParams.
func (BesuQBFTParams) ClientType() string { return ClientTypeBesuQBFT }

// BesuQBFTSource is implemented by targets whose chain runs Besu QBFT and can
// serve the consensus state of their head, which a new client tracking them
// starts trusting.
type BesuQBFTSource interface {
	BesuQBFTHead(ctx context.Context) (height uint64, state besumsgs.IBesuLightClientMsgsConsensusState, err error)
}
