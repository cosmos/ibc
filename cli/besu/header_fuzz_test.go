// SPDX-License-Identifier: Apache-2.0

package besu_test

import (
	"slices"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/besu"
	"github.com/cosmos/ibc/cli/besu/besutest"
	"github.com/cosmos/ibc/cli/internal/pkg/ecrecover"
)

// FuzzParseSealedHeader parses node-supplied headers. Whatever it accepts it
// must encode in the signing form of the only consensus any validator signed
// under, changing only the commit seals, keep one canonical 27/28 seal per
// validator that signed, in strictly ascending signer order, and parsing its
// own output must give the same result.
func FuzzParseSealedHeader(f *testing.F) {
	for _, consensus := range besu.Consensuses {
		fixture := besutest.MustFixture(f, consensus)
		for _, update := range []besutest.UpdateFixture{fixture.AdjacentUpdate, fixture.NonAdjacentUpdate} {
			encoded, err := rlp.EncodeToBytes(besutest.SealedHeader(f, update.HeaderRLP, update.CommitSeals))
			require.NoError(f, err)
			f.Add(encoded)
		}
	}

	f.Fuzz(func(t *testing.T, headerRLP []byte) {
		var header types.Header
		if rlp.DecodeBytes(headerRLP, &header) != nil {
			return
		}
		parsed, err := besu.ParseSealedHeader(&header)
		if err != nil {
			return
		}

		var signing types.Header
		require.NoError(t, rlp.DecodeBytes(parsed.RLP, &signing))
		require.Equal(t, header.Number.Uint64(), parsed.Height)
		require.Equal(t, header.Time, parsed.Timestamp)
		require.Equal(t, header.Root, parsed.StateRoot)

		require.Equal(t, withoutExtra(t, header), withoutExtra(t, signing), "header changed beyond extraData")
		inputItems, inputSeals := splitExtra(t, header.Extra)
		var signingItems []rlp.RawValue
		require.NoError(t, rlp.DecodeBytes(signing.Extra, &signingItems))
		require.GreaterOrEqual(t, len(signingItems), 4)
		require.Equal(t, inputItems[:4], signingItems[:4], "extraData changed beyond the commit seals")

		var validators []common.Address
		require.NoError(t, rlp.DecodeBytes(inputItems[1], &validators))
		signers := func(consensus besu.Consensus) []common.Address {
			digest, digestErr := besu.CommitSealDigest(&header, inputItems, consensus)
			require.NoError(t, digestErr)
			var signed []common.Address
			for _, seal := range inputSeals {
				signer, _, sigErr := ecrecover.Recover(digest, seal)
				if sigErr == nil && slices.Contains(validators, signer) {
					signed = append(signed, signer)
				}
			}
			slices.SortFunc(signed, common.Address.Cmp)
			return slices.Compact(signed)
		}
		for _, other := range besu.Consensuses {
			if other != parsed.Consensus {
				require.Empty(t, signers(other), "validators signed under %s, detected %s", other, parsed.Consensus)
			}
		}
		want := signers(parsed.Consensus)
		digest, err := besu.CommitSealDigest(&header, inputItems, parsed.Consensus)
		require.NoError(t, err)
		require.Equal(t, digest, crypto.Keccak256(parsed.RLP), "RLP is not the signing form")
		require.Equal(t, want, recoverSeals(t, parsed), "not one seal per validator that signed, in signer order")
		require.Equal(t, want, parsed.Signers)

		reparsed, err := besu.ParseSealedHeader(
			besutest.SealedHeader(t, parsed.RLP, besutest.CommitSeals(parsed.CommitSeals)),
		)
		require.NoError(t, err)
		require.Equal(t, parsed.RLP, reparsed.RLP, "parsing is not idempotent")
		require.Equal(t, parsed.CommitSeals, reparsed.CommitSeals)
		require.Equal(t, parsed.Consensus, reparsed.Consensus)
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
