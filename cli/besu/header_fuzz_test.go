// SPDX-License-Identifier: Apache-2.0

package besu

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/require"
)

// FuzzParseSealedHeader parses node-supplied headers. Whatever it accepts it
// must only reorder the commit seals, into strictly ascending signer order,
// and parsing its own output must give the same bytes.
func FuzzParseSealedHeader(f *testing.F) {
	f.Fuzz(func(t *testing.T, headerRLP []byte) {
		var header types.Header
		if rlp.DecodeBytes(headerRLP, &header) != nil {
			return
		}
		parsed, err := ParseSealedHeader(&header)
		if err != nil {
			return
		}

		var sorted types.Header
		require.NoError(t, rlp.DecodeBytes(parsed.RLP, &sorted))
		require.Equal(t, header.Number.Uint64(), parsed.Height)
		require.Equal(t, header.Time, parsed.Timestamp)
		require.Equal(t, header.Root, parsed.StateRoot)

		inputItems, inputSeals := splitExtra(t, header.Extra)
		sortedItems, sortedSeals := splitExtra(t, sorted.Extra)
		require.ElementsMatch(t, inputSeals, sortedSeals, "seals were changed, not just reordered")
		inputDigest, err := commitSealDigest(&header, inputItems)
		require.NoError(t, err)
		digest, err := commitSealDigest(&sorted, sortedItems)
		require.NoError(t, err)
		require.Equal(t, inputDigest, digest, "header changed beyond the seal order")

		var previous common.Address
		for i, seal := range sortedSeals {
			pubkey, sigErr := crypto.SigToPub(digest, seal)
			require.NoError(t, sigErr)
			signer := crypto.PubkeyToAddress(*pubkey)
			require.Positive(t, signer.Cmp(previous), "seal %d out of signer order", i)
			previous = signer
		}

		reparsed, err := ParseSealedHeader(&sorted)
		require.NoError(t, err)
		require.Equal(t, parsed.RLP, reparsed.RLP, "parsing is not idempotent")
	})
}

func splitExtra(t *testing.T, extra []byte) ([]rlp.RawValue, [][]byte) {
	t.Helper()

	var items []rlp.RawValue
	require.NoError(t, rlp.DecodeBytes(extra, &items))
	var seals [][]byte
	require.NoError(t, rlp.DecodeBytes(items[extraIdxCommitSeals], &seals))

	return items, seals
}
