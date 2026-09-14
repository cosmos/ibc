// SPDX-License-Identifier: Apache-2.0

package besutest

import (
	"fmt"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besumsgs"

	"github.com/cosmos/ibc/cli/besu"
)

// UpdateClient is the decoded form of an updateClient payload.
type UpdateClient struct {
	HeaderRLP              []byte
	TrustedHeight          uint64
	ConsensusStatePreimage besu.ConsensusState
	AccountProof           [][]byte
}

// MembershipProof is the decoded form of the proof bytes a verifyMembership or
// verifyNonMembership call carries.
type MembershipProof struct {
	ConsensusStatePreimage besu.ConsensusState
	ProofNodes             [][]byte
}

// DecodeProofNodes reverses besu.EncodeProofNodes.
func DecodeProofNodes(data []byte) ([][]byte, error) {
	nodes, err := besumsgs.DecodeProofNodes(data)
	if err != nil {
		return nil, fmt.Errorf("decode proof nodes: %w", err)
	}

	return nodes, nil
}

// DecodeUpdateClient reverses besu.EncodeUpdateClient, rejecting a non-zero
// revision number.
func DecodeUpdateClient(data []byte) (UpdateClient, error) {
	decoded, err := besumsgs.DecodeUpdateClient(data)
	if err != nil {
		return UpdateClient{}, fmt.Errorf("decode update client: %w", err)
	}

	if decoded.TrustedHeight.RevisionNumber != 0 {
		return UpdateClient{}, fmt.Errorf(
			"trusted revision number %d, want 0", decoded.TrustedHeight.RevisionNumber,
		)
	}

	nodes, err := DecodeProofNodes(decoded.AccountProof)
	if err != nil {
		return UpdateClient{}, err
	}

	return UpdateClient{
		HeaderRLP:              decoded.HeaderRlp,
		TrustedHeight:          decoded.TrustedHeight.RevisionHeight,
		ConsensusStatePreimage: consensusState(decoded.ConsensusStatePreimage),
		AccountProof:           nodes,
	}, nil
}

// DecodeMembershipProof reverses besu.EncodeMembershipProof.
func DecodeMembershipProof(data []byte) (MembershipProof, error) {
	decoded, err := besumsgs.DecodeMembershipProof(data)
	if err != nil {
		return MembershipProof{}, fmt.Errorf("decode membership proof: %w", err)
	}

	return MembershipProof{
		ConsensusStatePreimage: consensusState(decoded.ConsensusStatePreimage),
		ProofNodes:             decoded.ProofNodes,
	}, nil
}

// EncodeClientState produces getClientState() output, for tests and fakes.
func EncodeClientState(state besu.ClientState) ([]byte, error) {
	data, err := besumsgs.EncodeClientState(besumsgs.IBesuLightClientMsgsClientState{
		IbcRouter:      state.IBCRouter,
		LatestHeight:   besumsgs.IICS02ClientMsgsHeight{RevisionHeight: state.LatestHeight},
		TrustingPeriod: state.TrustingPeriod,
		MaxClockDrift:  state.MaxClockDrift,
	})
	if err != nil {
		return nil, fmt.Errorf("encode client state: %w", err)
	}

	return data, nil
}

func consensusState(state besumsgs.IBesuLightClientMsgsConsensusState) besu.ConsensusState {
	return besu.ConsensusState{Timestamp: state.Timestamp, StorageRoot: state.StorageRoot, Validators: state.Validators}
}
