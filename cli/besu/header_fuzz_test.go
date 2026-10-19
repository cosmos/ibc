// SPDX-License-Identifier: Apache-2.0

package besu_test

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/besu"
	"github.com/cosmos/ibc/cli/besu/besutest"
)

// FuzzParseSealedHeader parses node-supplied headers. Whatever it accepts it
// must only reorder the commit seals, into strictly ascending signer order,
// and parsing its own output must give the same bytes.
func FuzzParseSealedHeader(f *testing.F) {
	fixture := besutest.MustFixture(f)
	f.Add([]byte(fixture.AdjacentUpdate.HeaderRLP))
	f.Add([]byte(fixture.NonAdjacentUpdate.HeaderRLP))

	f.Fuzz(func(t *testing.T, headerRLP []byte) {
		var header types.Header
		if rlp.DecodeBytes(headerRLP, &header) != nil {
			return
		}
		parsed, err := besu.ParseSealedHeader(&header)
		if err != nil {
			return
		}

		var sorted types.Header
		require.NoError(t, rlp.DecodeBytes(parsed.RLP, &sorted))
		require.Equal(t, header.Number.Uint64(), parsed.Height)
		require.Equal(t, header.Time, parsed.Timestamp)
		require.Equal(t, header.Root, parsed.StateRoot)

		require.Equal(t, withoutExtra(t, header), withoutExtra(t, sorted), "header changed beyond extraData")
		inputItems, inputSeals := splitExtra(t, header.Extra)
		sortedItems, sortedSeals := splitExtra(t, sorted.Extra)
		require.Equal(t, inputItems[:4], sortedItems[:4], "extraData changed beyond the commit seals")
		require.ElementsMatch(t, inputSeals, sortedSeals, "seals were changed, not just reordered")

		digest, err := besu.CommitSealDigest(&header, inputItems)
		require.NoError(t, err)
		var previous common.Address
		for i, seal := range sortedSeals {
			pubkey, sigErr := crypto.SigToPub(digest, seal)
			require.NoError(t, sigErr)
			signer := crypto.PubkeyToAddress(*pubkey)
			require.Positive(t, signer.Cmp(previous), "seal %d out of signer order", i)
			previous = signer
		}

		reparsed, err := besu.ParseSealedHeader(&sorted)
		require.NoError(t, err)
		require.Equal(t, parsed.RLP, reparsed.RLP, "parsing is not idempotent")
	})
}

// withoutExtra is the RLP of header with extraData emptied.
func withoutExtra(t *testing.T, header types.Header) []byte {
	t.Helper()

	header.Extra = nil
	encoded, err := rlp.EncodeToBytes(&header)
	require.NoError(t, err)

	return encoded
}

func splitExtra(t *testing.T, extra []byte) ([]rlp.RawValue, [][]byte) {
	t.Helper()

	var items []rlp.RawValue
	require.NoError(t, rlp.DecodeBytes(extra, &items))
	require.Len(t, items, 5)
	var seals [][]byte
	require.NoError(t, rlp.DecodeBytes(items[4], &seals))

	return items, seals
}
