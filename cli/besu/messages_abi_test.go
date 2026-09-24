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

func TestConsensusStateHashMatchesSolidity(t *testing.T) {
	fixture := besutest.MustFixture(t)
	// keccak256(abi.encode(ConsensusState)) for qbft.json's initial trusted state,
	// computed independently: cast abi-encode "f((uint64,bytes32,address[]))" \
	//   "(<initialTrustedTimestamp>,<initialTrustedStateRoot>,[<initialTrustedValidators>])" | cast keccak
	got, err := besutest.HashConsensusState(fixture.InitialConsensusState())
	require.NoError(t, err)
	assert.Equal(t, common.HexToHash("0x91c4debaf593d0d6251ab85a28ff33ffdbb5cda3a070ab011402ba4599a2b66f"), got)
}

func TestCommitmentSlotMatchesSolidity(t *testing.T) {
	fixture := besutest.MustFixture(t)
	// the slot for qbft.json's membership path, computed independently:
	//   cast keccak "$(cast abi-encode 'f(bytes32,bytes32)' "$(cast keccak <path>)" <IbcStoreStorageSlot>)"
	assert.Equal(
		t,
		common.HexToHash("0x54dec64b8cfb867e4e0b052552b929bd5886439932474991c8895d84bcc8c6d9"),
		besu.CommitmentSlot(fixture.Membership.Path),
	)
}
