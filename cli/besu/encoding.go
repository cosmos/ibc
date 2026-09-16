// SPDX-License-Identifier: Apache-2.0

package besu

import (
	"fmt"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besumsgs"
)

var messageBindings = besumsgs.NewBindings()

// EncodeProofNodes is abi.encode(bytes[]) over ordered RLP trie nodes, the
// shape the captured fixtures store proofs in.
func EncodeProofNodes(nodes [][]byte) ([]byte, error) {
	data, err := messageBindings.TryPackProofNodes(nodes)
	if err != nil {
		return nil, fmt.Errorf("encode proof nodes: %w", err)
	}

	// Wire payloads contain ABI arguments without the schema function selector.
	return data[4:], nil
}

// EncodeUpdateClient builds the updateClient payload: the raw header, the
// trusted height (revision 0) and the preimage of the consensus state trusted
// at that height.
func EncodeUpdateClient(headerRLP []byte, trustedHeight uint64, preimage ConsensusState) ([]byte, error) {
	data, err := messageBindings.TryPackUpdateClient(besumsgs.IBesuLightClientMsgsMsgUpdateClient{
		HeaderRlp:              headerRLP,
		TrustedHeight:          besumsgs.IICS02ClientMsgsHeight{RevisionHeight: trustedHeight},
		ConsensusStatePreimage: preimage.toABI(),
	})
	if err != nil {
		return nil, fmt.Errorf("encode update client: %w", err)
	}

	return data[4:], nil
}

// EncodeMembershipProof builds the proof bytes for verifyMembership and
// verifyNonMembership: the preimage of the consensus state at the proof height,
// the state trie nodes proving the router account and the storage trie nodes
// eth_getProof returned for the commitment slot. accountProofNodes may be
// empty when an earlier call in the same transaction proved the same height.
func EncodeMembershipProof(preimage ConsensusState, accountProofNodes, proofNodes [][]byte) ([]byte, error) {
	data, err := messageBindings.TryPackMembershipProof(besumsgs.IBesuLightClientMsgsMembershipProof{
		ConsensusStatePreimage: preimage.toABI(),
		AccountProofNodes:      accountProofNodes,
		ProofNodes:             proofNodes,
	})
	if err != nil {
		return nil, fmt.Errorf("encode membership proof: %w", err)
	}

	return data[4:], nil
}
