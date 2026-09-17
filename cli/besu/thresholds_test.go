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

func TestThresholdFormulas(t *testing.T) {
	cases := []struct{ n, overlap, quorum int }{
		{1, 1, 1}, {2, 1, 2}, {3, 2, 2}, {4, 2, 3}, {5, 2, 4}, {6, 3, 4}, {7, 3, 5}, {9, 4, 6}, {10, 4, 7},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.overlap, besu.OverlapRequired(tc.n), "overlap n=%d", tc.n)
		assert.Equal(t, tc.quorum, besu.QuorumRequired(tc.n), "quorum n=%d", tc.n)
	}
}

func TestCheckUpdateThresholds(t *testing.T) {
	addrs := besutest.Addresses(besutest.Keys(7))
	six := addrs[:6]
	header := &besu.Header{Validators: six}
	trusted := besumsgs.IBesuLightClientMsgsConsensusState{Validators: six}

	// Four of six meets quorum; a signer outside the set contributes nothing.
	require.NoError(t, besu.CheckUpdate(header, append([]common.Address{addrs[6]}, six[:4]...), trusted))
	err := besu.CheckUpdate(header, six[:3], trusted)
	require.ErrorIs(t, err, besu.ErrInsufficientQuorum)
	require.ErrorContains(t, err, "3 signers in the header set, 4 required")

	// Three of six meets overlap, independently of the target's quorum.
	header.Validators = six[:3]
	require.NoError(t, besu.CheckUpdate(header, six[:3], trusted))
	err = besu.CheckUpdate(header, six[:2], trusted)
	require.ErrorIs(t, err, besu.ErrInsufficientOverlap)
	require.ErrorContains(t, err, "2 signers in the trusted set, 3 required")
}

func TestCheckUpdateFixtures(t *testing.T) {
	fixture := besutest.MustFixture(t)
	trusted := fixture.InitialConsensusState()

	check := func(update besutest.UpdateFixture) error {
		header, err := besu.ParseHeader(update.HeaderRLP)
		require.NoError(t, err)

		signers, err := header.Signers()
		require.NoError(t, err)

		return besu.CheckUpdate(header, signers, trusted)
	}

	require.NoError(t, check(fixture.AdjacentUpdate))
	require.NoError(t, check(fixture.NonAdjacentUpdate))

	err := check(fixture.LowQuorumUpdate)
	require.ErrorIs(t, err, besu.ErrInsufficientQuorum)

	require.ErrorContains(t, err, "3 required")

	err = check(fixture.LowOverlapUpdate)
	require.ErrorIs(t, err, besu.ErrInsufficientOverlap)

	require.ErrorContains(t, err, "2 required")
}

func TestCheckUpdateFlagsRLPFidelity(t *testing.T) {
	fixture := besutest.MustFixture(t)
	header, err := besu.ParseHeader(fixture.AdjacentUpdate.HeaderRLP)
	require.NoError(t, err)

	strangers := besutest.Addresses(besutest.Keys(3))

	err = besu.CheckUpdate(header, strangers, fixture.InitialConsensusState())
	require.ErrorIs(t, err, besu.ErrInsufficientOverlap)
	require.ErrorContains(t, err, "RLP fidelity")
}
