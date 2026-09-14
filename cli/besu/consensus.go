// SPDX-License-Identifier: Apache-2.0

package besu

import (
	"fmt"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besumsgs"
	"github.com/ethereum/go-ethereum/common"
)

// ConsensusState is what the light client trusts about one Besu height. The
// contract stores only its Hash, so callers resend the full value with every
// update and every membership proof.
type ConsensusState struct {
	Timestamp   uint64
	StorageRoot common.Hash
	Validators  []common.Address
}

// Hash is keccak256(abi.encode(ConsensusState)), the value the light client
// stores per height. The struct holds a dynamic array, so the encoding starts
// with an offset word.
func (c ConsensusState) Hash() (common.Hash, error) {
	return besumsgs.ConsensusStateHash(c.toABI())
}

func (c ConsensusState) toABI() besumsgs.IBesuLightClientMsgsConsensusState {
	return besumsgs.IBesuLightClientMsgsConsensusState{
		Timestamp: c.Timestamp, StorageRoot: c.StorageRoot, Validators: c.Validators,
	}
}

// ClientState mirrors the light client's ClientState with the always-zero
// revision number folded away.
type ClientState struct {
	IBCRouter      common.Address
	LatestHeight   uint64
	TrustingPeriod uint64
	MaxClockDrift  uint64
}

// clientStateEncodedLen is the size of abi.encode(ClientState): five static words.
const clientStateEncodedLen = 5 * 32

// DecodeClientState decodes getClientState() output strictly: exactly five
// words, revision number zero and a non-zero latest height.
func DecodeClientState(data []byte) (ClientState, error) {
	if len(data) != clientStateEncodedLen {
		return ClientState{}, fmt.Errorf(
			"%w: %d bytes, want %d", ErrInvalidClientState, len(data), clientStateEncodedLen,
		)
	}

	state, err := besumsgs.DecodeClientState(data)
	if err != nil {
		return ClientState{}, fmt.Errorf("%w: %w", ErrInvalidClientState, err)
	}

	if state.LatestHeight.RevisionNumber != 0 {
		return ClientState{}, fmt.Errorf(
			"%w: revision number %d, want 0", ErrInvalidClientState, state.LatestHeight.RevisionNumber,
		)
	}

	if state.LatestHeight.RevisionHeight == 0 {
		return ClientState{}, fmt.Errorf("%w: latest height is zero", ErrInvalidClientState)
	}

	return ClientState{
		IBCRouter:      state.IbcRouter,
		LatestHeight:   state.LatestHeight.RevisionHeight,
		TrustingPeriod: state.TrustingPeriod,
		MaxClockDrift:  state.MaxClockDrift,
	}, nil
}
