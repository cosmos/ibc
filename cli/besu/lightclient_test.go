// SPDX-License-Identifier: Apache-2.0

package besu_test

import (
	"testing"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besumsgs"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/besu"
	"github.com/cosmos/ibc/cli/besu/besutest"
)

func TestUpdateClientRoundTrip(t *testing.T) {
	fixture := besutest.MustFixture(t)
	update := fixture.NonAdjacentUpdate

	preimage := fixture.InitialConsensusState()

	encoded, err := besu.EncodeUpdateClient(besumsgs.IBesuLightClientMsgsMsgUpdateClient{
		HeaderRlp:              update.HeaderRLP,
		TrustedHeight:          besumsgs.IICS02ClientMsgsHeight{RevisionHeight: update.TrustedHeight},
		ConsensusStatePreimage: preimage,
	})
	require.NoError(t, err)

	decoded, err := besumsgs.NewBindings().UnpackUpdateClient(encoded)
	require.NoError(t, err)
	assert.Equal(t, []byte(update.HeaderRLP), decoded.HeaderRlp)
	assert.Equal(t, besumsgs.IICS02ClientMsgsHeight{RevisionHeight: update.TrustedHeight}, decoded.TrustedHeight)
	assert.Equal(t, preimage, decoded.ConsensusStatePreimage)
}

func TestMembershipProofRoundTrip(t *testing.T) {
	fixture := besutest.MustFixture(t)
	preimage := fixture.NonAdjacentUpdate.ExpectedConsensusState()

	for _, m := range []besutest.MembershipFixture{fixture.Membership, fixture.NonMembership} {
		nodes, err := m.ProofNodes()
		require.NoError(t, err)
		accountNodes, err := m.AccountProofNodes()
		require.NoError(t, err)

		encoded, err := besu.EncodeMembershipProof(besumsgs.IBesuLightClientMsgsMembershipProof{
			ConsensusStatePreimage: preimage,
			AccountProofNodes:      accountNodes,
			ProofNodes:             nodes,
		})
		require.NoError(t, err)

		decoded, err := besumsgs.NewBindings().UnpackMembershipProof(encoded)
		require.NoError(t, err)
		assert.Equal(t, preimage, decoded.ConsensusStatePreimage)
		assert.Equal(t, accountNodes, decoded.AccountProofNodes)
		assert.Equal(t, nodes, decoded.ProofNodes)

		cached, err := besu.EncodeMembershipProof(besumsgs.IBesuLightClientMsgsMembershipProof{
			ConsensusStatePreimage: preimage,
			ProofNodes:             nodes,
		})
		require.NoError(t, err)
		decoded, err = besumsgs.NewBindings().UnpackMembershipProof(cached)
		require.NoError(t, err)
		assert.Empty(t, decoded.AccountProofNodes)
	}
}

func TestCommitmentSlotMatchesSolidity(t *testing.T) {
	// the packet commitment path of client-0 sequence 1, computed independently:
	//   cast keccak "$(cast abi-encode 'f(bytes32,bytes32)' "$(cast keccak <path>)" <IbcStoreStorageSlot>)"
	path := common.FromHex("0x636c69656e742d30010000000000000001")
	assert.Equal(
		t,
		common.HexToHash("0x79a5307f4993e7eb3894d3aafa592f86bb1b605ca1a506e595f1874343b9a987"),
		besu.CommitmentSlot(path),
	)
}
