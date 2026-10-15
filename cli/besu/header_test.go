// SPDX-License-Identifier: Apache-2.0

package besu

import (
	"slices"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/besu/besutest"
)

func TestParseSealedHeaderFixture(t *testing.T) {
	fixture := besutest.MustFixture(t)

	for _, update := range []besutest.UpdateFixture{fixture.AdjacentUpdate, fixture.NonAdjacentUpdate} {
		var header types.Header
		require.NoError(t, rlp.DecodeBytes(update.HeaderRLP, &header))
		parsed, err := ParseSealedHeader(&header)
		require.NoError(t, err)

		assert.Equal(t, update.Height, parsed.Height)
		assert.Equal(t, update.ExpectedTimestamp, parsed.Timestamp)
		assert.Equal(t, update.ExpectedStateRoot, parsed.StateRoot)
		assert.Equal(t, update.ExpectedValidators, parsed.Validators)
		assert.Equal(t, []byte(update.HeaderRLP), parsed.RLP)
	}
}

func TestParseSealedHeaderSortsCommitSeals(t *testing.T) {
	fixture := besutest.MustFixture(t)

	for _, update := range []besutest.UpdateFixture{fixture.AdjacentUpdate, fixture.NonAdjacentUpdate} {
		var header types.Header
		require.NoError(t, rlp.DecodeBytes(update.HeaderRLP, &header))

		extraItems, seals := splitExtra(t, header.Extra)
		require.Greater(t, len(seals), 1)
		slices.Reverse(seals)
		setSeals(t, &header, extraItems, seals)

		parsed, err := ParseSealedHeader(&header)
		require.NoError(t, err)
		assert.Equal(t, []byte(update.HeaderRLP), parsed.RLP)
	}
}

func TestParseSealedHeaderRejectsRepeatedSigner(t *testing.T) {
	var header types.Header
	require.NoError(t, rlp.DecodeBytes(besutest.MustFixture(t).AdjacentUpdate.HeaderRLP, &header))
	extraItems, seals := splitExtra(t, header.Extra)
	setSeals(t, &header, extraItems, append(seals, seals[0]))

	_, err := ParseSealedHeader(&header)
	require.ErrorContains(t, err, "more than one commit seal")
}

func TestParseSealedHeaderRejectsMalformed(t *testing.T) {
	var header types.Header
	require.NoError(t, rlp.DecodeBytes(besutest.MustFixture(t).AdjacentUpdate.HeaderRLP, &header))

	_, err := ParseSealedHeader(nil)
	require.ErrorContains(t, err, "nil header")

	header.Extra = []byte{0x80}
	_, err = ParseSealedHeader(&header)
	require.ErrorContains(t, err, "extra data list")

	header.Extra, err = rlp.EncodeToBytes([][]byte{{}, {}, {}, {}})
	require.NoError(t, err)
	_, err = ParseSealedHeader(&header)
	require.ErrorContains(t, err, "4 extra data items, want 5")
}

// TestCommitSealDigestRecoversValidators checks the digest against the
// header's own validator set: with a wrong digest the fixture's seals would
// recover to addresses outside it.
func TestCommitSealDigestRecoversValidators(t *testing.T) {
	fixture := besutest.MustFixture(t)

	for _, update := range []besutest.UpdateFixture{fixture.AdjacentUpdate, fixture.NonAdjacentUpdate} {
		var header types.Header
		require.NoError(t, rlp.DecodeBytes(update.HeaderRLP, &header))
		extraItems, seals := splitExtra(t, header.Extra)
		require.NotEmpty(t, seals)

		digest, err := commitSealDigest(&header, extraItems)
		require.NoError(t, err)

		var previous common.Address
		for i, seal := range seals {
			pubkey, err := crypto.SigToPub(digest, seal)
			require.NoError(t, err)
			signer := crypto.PubkeyToAddress(*pubkey)
			require.Contains(t, update.ExpectedValidators, signer, "seal %d", i)
			// The fixture is in the order the contract accepts.
			require.Positive(t, signer.Cmp(previous), "seal %d out of order", i)
			previous = signer
		}
	}
}

func setSeals(t *testing.T, header *types.Header, extraItems []rlp.RawValue, seals [][]byte) {
	t.Helper()

	var err error
	extraItems[extraIdxCommitSeals], err = rlp.EncodeToBytes(seals)
	require.NoError(t, err)
	header.Extra, err = rlp.EncodeToBytes(extraItems)
	require.NoError(t, err)
}
