// SPDX-License-Identifier: Apache-2.0

package besu_test

import (
	"bytes"
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

	encoded, err := besu.EncodeUpdateClient(update.HeaderRLP, update.TrustedHeight, preimage)
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

		encoded, err := besu.EncodeMembershipProof(preimage, accountNodes, nodes)
		require.NoError(t, err)

		decoded, err := besumsgs.NewBindings().UnpackMembershipProof(encoded)
		require.NoError(t, err)
		assert.Equal(t, preimage, decoded.ConsensusStatePreimage)
		assert.Equal(t, accountNodes, decoded.AccountProofNodes)
		assert.Equal(t, nodes, decoded.ProofNodes)

		cached, err := besu.EncodeMembershipProof(preimage, nil, nodes)
		require.NoError(t, err)
		decoded, err = besumsgs.NewBindings().UnpackMembershipProof(cached)
		require.NoError(t, err)
		assert.Empty(t, decoded.AccountProofNodes)
	}
}

func TestClientStateRoundTrip(t *testing.T) {
	state := besumsgs.IBesuLightClientMsgsClientState{
		IbcRouter:      common.HexToAddress("0xe2beCC7d4F673682BedA1AD6D7186784C3D43b2F"),
		LatestHeight:   besumsgs.IICS02ClientMsgsHeight{RevisionHeight: 114},
		TrustingPeriod: 1209600,
		MaxClockDrift:  15,
	}

	encoded, err := besutest.EncodeClientState(state)
	require.NoError(t, err)
	require.Len(t, encoded, 5*32, "ClientState is fully static")

	decoded, err := besumsgs.NewBindings().UnpackClientState(encoded)
	require.NoError(t, err)
	assert.Equal(t, state, decoded)
}

func TestConsensusStateHashChangesWithEveryField(t *testing.T) {
	fixture := besutest.MustFixture(t)
	base := fixture.InitialConsensusState()

	h0, err := besu.HashConsensusState(base)
	require.NoError(t, err)

	ts := base
	ts.Timestamp++
	h1, err := besu.HashConsensusState(ts)
	require.NoError(t, err)

	root := base
	root.StateRoot[0] ^= 1
	h2, err := besu.HashConsensusState(root)
	require.NoError(t, err)

	vals := base
	vals.Validators = base.Validators[:3]
	h3, err := besu.HashConsensusState(vals)
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
			_, err := besumsgs.NewBindings().UnpackProofNodes(data)
			require.Error(t, err)
			_, err = besumsgs.NewBindings().UnpackUpdateClient(data)
			require.Error(t, err)
			_, err = besumsgs.NewBindings().UnpackMembershipProof(data)
			require.Error(t, err)
		})
	}
}

func TestConsensusStateHashMatchesSolidity(t *testing.T) {
	fixture := besutest.MustFixture(t)
	// keccak256(abi.encode(ConsensusState)) for qbft.json's initial trusted state,
	// computed independently with `cast abi-encode ... | cast keccak`.
	got, err := besu.HashConsensusState(fixture.InitialConsensusState())
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

func TestGeneratedEncodingSchema(t *testing.T) {
	bindings := besumsgs.NewBindings()
	for name, method := range bindings.GetABI().Methods {
		t.Run(name, func(t *testing.T) {
			require.Len(t, method.Inputs, 1)
			require.Len(t, method.Outputs, 1)
			assert.Equal(t, method.Inputs[0].Type.String(), method.Outputs[0].Type.String())
		})
	}
	state := besutest.MustFixture(t).InitialConsensusState()
	encoded, err := bindings.TryPackConsensusState(state)
	require.NoError(t, err)
	decoded, err := bindings.UnpackConsensusState(encoded[4:])
	require.NoError(t, err)
	assert.Equal(t, state, decoded)

	_, err = bindings.UnpackConsensusState(nil)
	require.Error(t, err)
}
