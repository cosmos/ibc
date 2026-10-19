// SPDX-License-Identifier: Apache-2.0

package besu_test

import (
	"math/big"
	"slices"
	"testing"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/besu"
	"github.com/cosmos/ibc/cli/besu/besutest"
)

var consensuses = []besu.Consensus{besu.QBFT, besu.IBFT2}

func TestParseSealedHeaderFixture(t *testing.T) {
	for _, consensus := range consensuses {
		fixture := besutest.MustFixture(t, consensus)

		for _, update := range []besutest.UpdateFixture{fixture.AdjacentUpdate, fixture.NonAdjacentUpdate} {
			header := fixture.ParseHeader(t, update.HeaderRLP)

			assert.Equal(t, update.Height, header.Height)
			assert.Equal(t, update.ExpectedTimestamp, header.Timestamp)
			assert.Equal(t, update.ExpectedStateRoot, header.StateRoot)
			assert.Equal(t, update.ExpectedValidators, header.Validators)
			assert.Equal(t, []byte(update.HeaderRLP), header.RLP)
		}
	}
}

// Under the other consensus's digest no seal recovers to a validator, so a
// client type that does not match the chain fails at parse time.
func TestParseSealedHeaderRejectsOtherConsensus(t *testing.T) {
	for consensus, other := range map[besu.Consensus]besu.Consensus{besu.QBFT: besu.IBFT2, besu.IBFT2: besu.QBFT} {
		var header types.Header
		require.NoError(t, rlp.DecodeBytes(besutest.MustFixture(t, consensus).AdjacentUpdate.HeaderRLP, &header))

		_, err := besu.ParseSealedHeader(&header, other)
		require.ErrorContains(
			t,
			err,
			"no commit seal recovers to a header validator",
			"%s header as %s",
			consensus,
			other,
		)
	}
}

// The light client accepts only canonical seals from validators, one per
// signer, in strictly ascending signer order. Each mutation breaks that without
// changing which validators signed, so parsing must restore the sealed header.
func TestParseSealedHeaderNormalizesCommitSeals(t *testing.T) {
	for _, consensus := range consensuses {
		testParseSealedHeaderNormalizesCommitSeals(t, besutest.MustFixture(t, consensus))
	}
}

func testParseSealedHeaderNormalizesCommitSeals(t *testing.T, fixture besutest.Fixture) {
	t.Helper()
	updates := map[string]besutest.UpdateFixture{
		"adjacent":    fixture.AdjacentUpdate,
		"nonAdjacent": fixture.NonAdjacentUpdate,
	}
	for updateName, update := range updates {
		var header types.Header
		require.NoError(t, rlp.DecodeBytes(update.HeaderRLP, &header))
		items, _ := splitExtra(t, header.Extra)
		digest, err := besu.CommitSealDigest(&header, items, fixture.Consensus)
		require.NoError(t, err)
		key, err := crypto.GenerateKey()
		require.NoError(t, err)
		outsider, err := crypto.Sign(digest, key)
		require.NoError(t, err)

		highS := func(seals [][]byte) [][]byte {
			n := crypto.S256().Params().N
			new(big.Int).Sub(n, new(big.Int).SetBytes(seals[0][32:64])).FillBytes(seals[0][32:64])
			seals[0][64] ^= 1
			return seals
		}
		legacyV := func(seals [][]byte) [][]byte {
			seals[0][64] += 27
			return seals
		}
		repeated := func(seals [][]byte) [][]byte { return append(seals, seals[0]) }
		nonValidator := func(seals [][]byte) [][]byte { return append(seals, outsider) }
		invalid := func(seals [][]byte) [][]byte {
			return append(seals, []byte{0x01}, append(slices.Clone(seals[1][:64]), 2))
		}
		reversed := func(seals [][]byte) [][]byte {
			slices.Reverse(seals)
			return seals
		}
		mutations := map[string]func([][]byte) [][]byte{
			"highS":        highS,
			"legacyV":      legacyV,
			"repeated":     repeated,
			"nonValidator": nonValidator,
			"invalid":      invalid,
			"reversed":     reversed,
			"all": func(seals [][]byte) [][]byte {
				return reversed(invalid(nonValidator(append(legacyV(highS(seals)), seals[1]))))
			},
		}

		for name, mutate := range mutations {
			t.Run(string(fixture.Consensus)+"/"+updateName+"/"+name, func(t *testing.T) {
				var header types.Header
				require.NoError(t, rlp.DecodeBytes(update.HeaderRLP, &header))
				items, seals := splitExtra(t, header.Extra)
				require.Greater(t, len(seals), 1)
				var err error
				items[4], err = rlp.EncodeToBytes(mutate(seals))
				require.NoError(t, err)
				header.Extra, err = rlp.EncodeToBytes(items)
				require.NoError(t, err)

				parsed, err := besu.ParseSealedHeader(&header, fixture.Consensus)
				require.NoError(t, err)
				assert.Equal(t, []byte(update.HeaderRLP), parsed.RLP)
			})
		}
	}
}

// Dropping every seal, as a seal digest mismatch would, must fail clearly. A
// header without seals, like the genesis block, still parses.
func TestParseSealedHeaderRequiresAValidatorSeal(t *testing.T) {
	var header types.Header
	require.NoError(t, rlp.DecodeBytes(besutest.MustFixture(t, besu.QBFT).AdjacentUpdate.HeaderRLP, &header))
	items, seals := splitExtra(t, header.Extra)
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	outsider, err := crypto.Sign(crypto.Keccak256([]byte("another digest")), key)
	require.NoError(t, err)

	withSeals := func(seals [][]byte) *types.Header {
		encoded, encErr := rlp.EncodeToBytes(seals)
		require.NoError(t, encErr)
		items[4] = encoded
		sealed := types.CopyHeader(&header)
		sealed.Extra, encErr = rlp.EncodeToBytes(items)
		require.NoError(t, encErr)
		return sealed
	}

	_, err = besu.ParseSealedHeader(withSeals([][]byte{outsider, seals[0][:64]}), besu.QBFT)
	require.ErrorContains(t, err, "no commit seal recovers to a header validator")

	_, err = besu.ParseSealedHeader(withSeals([][]byte{}), besu.QBFT)
	require.NoError(t, err)
}

func TestParseSealedHeaderRejectsMalformed(t *testing.T) {
	var header types.Header
	require.NoError(t, rlp.DecodeBytes(besutest.MustFixture(t, besu.QBFT).AdjacentUpdate.HeaderRLP, &header))

	_, err := besu.ParseSealedHeader(nil, besu.QBFT)
	require.ErrorContains(t, err, "nil header")

	_, err = besu.ParseSealedHeader(&header, "clique")
	require.ErrorContains(t, err, `unknown besu consensus "clique"`)

	header.Extra = []byte{0x80}
	_, err = besu.ParseSealedHeader(&header, besu.QBFT)
	require.ErrorContains(t, err, "extra data list")

	header.Extra, err = rlp.EncodeToBytes([][]byte{{}, {}, {}, {}})
	require.NoError(t, err)
	_, err = besu.ParseSealedHeader(&header, besu.IBFT2)
	require.ErrorContains(t, err, "4 extra data items, want 5")
}
