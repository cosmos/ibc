// SPDX-License-Identifier: Apache-2.0

package besu_test

import (
	"bytes"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/besu"
	"github.com/cosmos/ibc/cli/besu/besutest"
)

func TestProofNodesRoundTrip(t *testing.T) {
	fixture := besutest.MustFixture(t)

	nodes, err := fixture.Membership.AccountProofNodes()
	require.NoError(t, err)
	require.NotEmpty(t, nodes)

	encoded, err := besu.EncodeProofNodes(nodes)
	require.NoError(t, err)
	assert.Equal(t, []byte(fixture.Membership.AccountProof), encoded)

	empty, err := besu.EncodeProofNodes(nil)
	require.NoError(t, err)

	decoded, err := besutest.DecodeProofNodes(empty)
	require.NoError(t, err)
	assert.Empty(t, decoded)
}

func TestUpdateClientRoundTrip(t *testing.T) {
	fixture := besutest.MustFixture(t)
	update := fixture.NonAdjacentUpdate

	preimage := fixture.InitialConsensusState()

	encoded, err := besu.EncodeUpdateClient(update.HeaderRLP, update.TrustedHeight, preimage)
	require.NoError(t, err)

	decoded, err := besutest.DecodeUpdateClient(encoded)
	require.NoError(t, err)
	assert.Equal(t, []byte(update.HeaderRLP), decoded.HeaderRLP)
	assert.Equal(t, update.TrustedHeight, decoded.TrustedHeight)
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

		encoded, err := besu.EncodeMembershipProof(preimage, accountNodes, nodes)
		require.NoError(t, err)

		decoded, err := besutest.DecodeMembershipProof(encoded)
		require.NoError(t, err)
		assert.Equal(t, preimage, decoded.ConsensusStatePreimage)
		assert.Equal(t, accountNodes, decoded.AccountProofNodes)
		assert.Equal(t, nodes, decoded.ProofNodes)

		cached, err := besu.EncodeMembershipProof(preimage, nil, nodes)
		require.NoError(t, err)
		decoded, err = besutest.DecodeMembershipProof(cached)
		require.NoError(t, err)
		assert.Empty(t, decoded.AccountProofNodes)
	}
}

func TestClientStateRoundTripAndStrictness(t *testing.T) {
	state := besu.ClientState{
		IBCRouter:      common.HexToAddress("0xe2beCC7d4F673682BedA1AD6D7186784C3D43b2F"),
		LatestHeight:   114,
		TrustingPeriod: 1209600,
		MaxClockDrift:  15,
	}

	encoded, err := besutest.EncodeClientState(state)
	require.NoError(t, err)
	require.Len(t, encoded, 5*32, "ClientState is fully static")

	decoded, err := besu.DecodeClientState(encoded)
	require.NoError(t, err)
	assert.Equal(t, state, decoded)

	_, err = besu.DecodeClientState(encoded[:len(encoded)-1])
	require.ErrorIs(t, err, besu.ErrInvalidClientState)

	revision := append([]byte(nil), encoded...)
	revision[63] = 1 // revisionNumber word
	_, err = besu.DecodeClientState(revision)
	require.ErrorIs(t, err, besu.ErrInvalidClientState)

	zeroHeight, err := besutest.EncodeClientState(besu.ClientState{IBCRouter: state.IBCRouter})
	require.NoError(t, err)
	_, err = besu.DecodeClientState(zeroHeight)
	require.ErrorIs(t, err, besu.ErrInvalidClientState)
}

func TestConsensusStateHashChangesWithEveryField(t *testing.T) {
	fixture := besutest.MustFixture(t)
	base := fixture.InitialConsensusState()

	h0, err := base.Hash()
	require.NoError(t, err)

	ts := base
	ts.Timestamp++
	h1, err := ts.Hash()
	require.NoError(t, err)

	root := base
	root.StateRoot[0] ^= 1
	h2, err := root.Hash()
	require.NoError(t, err)

	vals := base
	vals.Validators = base.Validators[:3]
	h3, err := vals.Hash()
	require.NoError(t, err)

	assert.NotEqual(t, h0, h1)
	assert.NotEqual(t, h0, h2)
	assert.NotEqual(t, h0, h3)
}

func TestPayloadDecodersRejectMalformedData(t *testing.T) {
	for name, data := range map[string][]byte{
		"empty":          nil,
		"short word":     {0},
		"invalid offset": bytes.Repeat([]byte{0xff}, 32),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := besutest.DecodeProofNodes(data)
			require.Error(t, err)
			_, err = besutest.DecodeUpdateClient(data)
			require.Error(t, err)
			_, err = besutest.DecodeMembershipProof(data)
			require.Error(t, err)
		})
	}
}

func TestConsensusStateHashMatchesSolidity(t *testing.T) {
	fixture := besutest.MustFixture(t)
	// keccak256(abi.encode(ConsensusState)) for qbft.json's initial trusted state,
	// computed independently with `cast abi-encode ... | cast keccak`.
	got, err := fixture.InitialConsensusState().Hash()
	require.NoError(t, err)
	assert.Equal(t, common.HexToHash("0x6ad73b19daaa61fcfc6d16fb89695b52ab719cc0348d014fd7cac8c1fd102bda"), got)
}

func TestCommitmentSlotMatchesSolidity(t *testing.T) {
	fixture := besutest.MustFixture(t)
	assert.Equal(
		t,
		common.HexToHash("0x54dec64b8cfb867e4e0b052552b929bd5886439932474991c8895d84bcc8c6d9"),
		besu.CommitmentSlot(fixture.Membership.Path),
	)
}

func TestUpdateClientRejectsNonzeroRevision(t *testing.T) {
	encoded, err := besu.EncodeUpdateClient(nil, 1, besu.ConsensusState{})
	require.NoError(t, err)
	// The tuple offset and header offset precede trustedHeight.revisionNumber.
	encoded[3*32-1] = 1
	_, err = besutest.DecodeUpdateClient(encoded)
	require.ErrorContains(t, err, "trusted revision number 1, want 0")
}
