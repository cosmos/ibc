// SPDX-License-Identifier: Apache-2.0

package besu

import (
	"fmt"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besumsgs"
)

var messageBindings = besumsgs.NewBindings()

// EncodeProofNodes is abi.encode(bytes[]) over ordered RLP trie nodes, the
// shape MsgUpdateClient.accountProof uses.
func EncodeProofNodes(nodes [][]byte) ([]byte, error) {
	data, err := messageBindings.TryPackProofNodes(nodes)
	if err != nil {
		return nil, fmt.Errorf("encode proof nodes: %w", err)
	}

	// Wire payloads contain ABI arguments without the schema function selector.
	return data[4:], nil
}

// EncodeUpdateClient builds the updateClient payload: the raw header, the
// trusted height (revision 0), the preimage of the consensus state trusted at
// that height and the router account proof nodes.
func EncodeUpdateClient(
	headerRLP []byte,
	trustedHeight uint64,
	preimage ConsensusState,
	accountProof [][]byte,
) ([]byte, error) {
	encodedProof, err := EncodeProofNodes(accountProof)
	if err != nil {
		return nil, err
	}

	data, err := messageBindings.TryPackUpdateClient(besumsgs.IBesuLightClientMsgsMsgUpdateClient{
		HeaderRlp:              headerRLP,
		TrustedHeight:          besumsgs.IICS02ClientMsgsHeight{RevisionHeight: trustedHeight},
		ConsensusStatePreimage: preimage.toABI(),
		AccountProof:           encodedProof,
	})
	if err != nil {
		return nil, fmt.Errorf("encode update client: %w", err)
	}

	return data[4:], nil
}

// EncodeMembershipProof builds the proof bytes for verifyMembership and
// verifyNonMembership: the preimage of the consensus state at the proof height
// and the storage trie nodes eth_getProof returned for the commitment slot.
func EncodeMembershipProof(preimage ConsensusState, proofNodes [][]byte) ([]byte, error) {
	data, err := messageBindings.TryPackMembershipProof(besumsgs.IBesuLightClientMsgsMembershipProof{
		ConsensusStatePreimage: preimage.toABI(),
		ProofNodes:             proofNodes,
	})
	if err != nil {
		return nil, fmt.Errorf("encode membership proof: %w", err)
	}

	return data[4:], nil
}
