// SPDX-License-Identifier: Apache-2.0

package besu

import (
	"fmt"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besumsgs"
)

var messageBindings = besumsgs.NewBindings()

// EncodeUpdateClient encodes the updateClient payload.
func EncodeUpdateClient(msg besumsgs.IBesuLightClientMsgsMsgUpdateClient) ([]byte, error) {
	data, err := messageBindings.TryPackUpdateClient(msg)
	if err != nil {
		return nil, fmt.Errorf("encode update client: %w", err)
	}

	return data[4:], nil
}

// EncodeMembershipProof encodes the proof bytes for verifyMembership and
// verifyNonMembership. AccountProofNodes may be empty when an earlier call in
// the same transaction proved the same height.
func EncodeMembershipProof(proof besumsgs.IBesuLightClientMsgsMembershipProof) ([]byte, error) {
	data, err := messageBindings.TryPackMembershipProof(proof)
	if err != nil {
		return nil, fmt.Errorf("encode membership proof: %w", err)
	}

	return data[4:], nil
}
