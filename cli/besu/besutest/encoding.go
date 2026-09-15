// SPDX-License-Identifier: Apache-2.0

package besutest

import (
	"fmt"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besumsgs"

	"github.com/cosmos/ibc/cli/besu"
)

var messageBindings = besumsgs.NewBindings()

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
	var decoded struct {
		Nodes [][]byte
	}
	if err := decodeArguments("proofNodes", data, &decoded); err != nil {
		return nil, fmt.Errorf("decode proof nodes: %w", err)
	}

	return decoded.Nodes, nil
}

// DecodeUpdateClient reverses besu.EncodeUpdateClient, rejecting a non-zero
// revision number.
func DecodeUpdateClient(data []byte) (UpdateClient, error) {
	var arguments struct {
		Message besumsgs.IBesuLightClientMsgsMsgUpdateClient
	}
	if err := decodeArguments("updateClient", data, &arguments); err != nil {
		return UpdateClient{}, fmt.Errorf("decode update client: %w", err)
	}

	decoded := arguments.Message
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
	var arguments struct {
		Proof besumsgs.IBesuLightClientMsgsMembershipProof
	}
	if err := decodeArguments("membershipProof", data, &arguments); err != nil {
		return MembershipProof{}, fmt.Errorf("decode membership proof: %w", err)
	}

	decoded := arguments.Proof
	return MembershipProof{
		ConsensusStatePreimage: consensusState(decoded.ConsensusStatePreimage),
		ProofNodes:             decoded.ProofNodes,
	}, nil
}

// EncodeClientState produces getClientState() output, for tests and fakes.
func EncodeClientState(state besu.ClientState) ([]byte, error) {
	data, err := messageBindings.TryPackClientState(besumsgs.IBesuLightClientMsgsClientState{
		IbcRouter:      state.IBCRouter,
		LatestHeight:   besumsgs.IICS02ClientMsgsHeight{RevisionHeight: state.LatestHeight},
		TrustingPeriod: state.TrustingPeriod,
		MaxClockDrift:  state.MaxClockDrift,
	})
	if err != nil {
		return nil, fmt.Errorf("encode client state: %w", err)
	}

	return data[4:], nil
}

func consensusState(state besumsgs.IBesuLightClientMsgsConsensusState) besu.ConsensusState {
	return besu.ConsensusState{Timestamp: state.Timestamp, StorageRoot: state.StorageRoot, Validators: state.Validators}
}

func decodeArguments(method string, data []byte, destination any) error {
	inputs := messageBindings.GetABI().Methods[method].Inputs
	values, err := inputs.Unpack(data)
	if err != nil {
		return err
	}

	return inputs.Copy(destination, values)
}
