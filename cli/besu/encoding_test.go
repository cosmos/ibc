// SPDX-License-Identifier: Apache-2.0

package besu_test

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/besu"
	"github.com/cosmos/ibc/cli/besu/besutest"
)

func TestProofNodesRoundTrip(t *testing.T) {
	fixture := besutest.MustFixture(t)

	nodes, err := fixture.NonAdjacentUpdate.AccountProofNodes()
	require.NoError(t, err)
	require.NotEmpty(t, nodes)

	encoded, err := besu.EncodeProofNodes(nodes)
	require.NoError(t, err)
	assert.Equal(t, []byte(fixture.NonAdjacentUpdate.AccountProof), encoded)

	empty, err := besu.EncodeProofNodes(nil)
	require.NoError(t, err)

	decoded, err := besutest.DecodeProofNodes(empty)
	require.NoError(t, err)
	assert.Empty(t, decoded)
}

func TestUpdateClientRoundTrip(t *testing.T) {
	fixture := besutest.MustFixture(t)
	update := fixture.NonAdjacentUpdate

	nodes, err := update.AccountProofNodes()
	require.NoError(t, err)

	preimage := fixture.InitialConsensusState()

	encoded, err := besu.EncodeUpdateClient(update.HeaderRLP, update.TrustedHeight, preimage, nodes)
	require.NoError(t, err)

	decoded, err := besutest.DecodeUpdateClient(encoded)
	require.NoError(t, err)
	assert.Equal(t, []byte(update.HeaderRLP), decoded.HeaderRLP)
	assert.Equal(t, update.TrustedHeight, decoded.TrustedHeight)
	assert.Equal(t, preimage, decoded.ConsensusStatePreimage)
	assert.Equal(t, nodes, decoded.AccountProof)
}

func TestMembershipProofRoundTrip(t *testing.T) {
	fixture := besutest.MustFixture(t)
	preimage := fixture.NonAdjacentUpdate.ExpectedConsensusState()

	for _, m := range []besutest.MembershipFixture{fixture.Membership, fixture.NonMembership} {
		nodes, err := m.ProofNodes()
		require.NoError(t, err)

		encoded, err := besu.EncodeMembershipProof(preimage, nodes)
		require.NoError(t, err)

		decoded, err := besutest.DecodeMembershipProof(encoded)
		require.NoError(t, err)
		assert.Equal(t, preimage, decoded.ConsensusStatePreimage)
		assert.Equal(t, nodes, decoded.ProofNodes)
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
	root.StorageRoot[0] ^= 1
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
