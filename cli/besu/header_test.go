// SPDX-License-Identifier: Apache-2.0

package besu_test

import (
	"crypto/ecdsa"
	"math/big"
	"slices"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/besu"
	"github.com/cosmos/ibc/cli/besu/besutest"
	"github.com/cosmos/ibc/cli/internal/pkg/ecrecover"
)

// Each fixture chain's headers parse to the consensus it runs: under the other
// consensus's digest no seal recovers to a validator.
func TestParseSealedHeaderFixture(t *testing.T) {
	for _, consensus := range besu.Consensuses {
		fixture := besutest.MustFixture(t, consensus)

		for _, update := range []besutest.UpdateFixture{fixture.AdjacentUpdate, fixture.NonAdjacentUpdate} {
			header := besutest.ParseHeader(t, update.HeaderRLP, update.CommitSeals)

			assert.Equal(t, consensus, header.Consensus)

			assert.Equal(t, update.Height, header.Height)
			assert.Equal(t, update.ExpectedTimestamp, header.Timestamp)
			assert.Equal(t, update.ExpectedStateRoot, header.StateRoot)
			assert.Equal(t, update.ExpectedValidators, header.Validators)
			assert.Equal(t, []byte(update.HeaderRLP), header.RLP)
			assert.Equal(t, besutest.Seals(update.CommitSeals), header.CommitSeals)
			assert.Equal(t, header.Signers, recoverSeals(t, header))
		}
	}
}

// The light client accepts only canonical seals from validators, one per
// signer, in strictly ascending signer order. Each mutation breaks that without
// changing which validators signed, so parsing must restore the sealed header.
func TestParseSealedHeaderNormalizesCommitSeals(t *testing.T) {
	for _, consensus := range besu.Consensuses {
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
		header := besutest.SealedHeader(t, update.HeaderRLP, update.CommitSeals)
		items, _ := splitExtra(t, header.Extra)
		digest, err := besu.CommitSealDigest(header, items, fixture.Consensus)
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
				header := besutest.SealedHeader(t, update.HeaderRLP, update.CommitSeals)
				items, seals := splitExtra(t, header.Extra)
				require.Greater(t, len(seals), 1)
				var err error
				items[4], err = rlp.EncodeToBytes(mutate(seals))
				require.NoError(t, err)
				header.Extra, err = rlp.EncodeToBytes(items)
				require.NoError(t, err)

				parsed, err := besu.ParseSealedHeader(header)
				require.NoError(t, err)
				assert.Equal(t, []byte(update.HeaderRLP), parsed.RLP)
				assert.Equal(t, besutest.Seals(update.CommitSeals), parsed.CommitSeals)
				assert.Equal(t, fixture.Consensus, parsed.Consensus)
				assert.Equal(t, besutest.ParseHeader(t, update.HeaderRLP, update.CommitSeals).Signers, parsed.Signers)
			})
		}
	}
}

// Dropping every seal, as seals from outside the validator set would, must
// fail clearly, as must a header without seals, like the genesis block.
func TestParseSealedHeaderRequiresAValidatorSeal(t *testing.T) {
	header := *fixtureHeader(t, besu.QBFT)
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

	_, err = besu.ParseSealedHeader(withSeals([][]byte{outsider, seals[0][:64]}))
	require.ErrorContains(t, err, "no commit seal recovers to a header validator")

	_, err = besu.ParseSealedHeader(withSeals([][]byte{}))
	require.ErrorContains(t, err, "no commit seals")
}

// A header resealed by a quorum of new validators parses to their consensus
// with those validators in ascending order and one seal per signer. A signer
// outside the validators, or repeated, is rejected.
func TestSealHeader(t *testing.T) {
	for _, consensus := range besu.Consensuses {
		header := *fixtureHeader(t, consensus)
		header.Root = common.HexToHash("0x01")

		keys := make([]*ecdsa.PrivateKey, 4)
		validators := make([]common.Address, len(keys))
		for i := range keys {
			key, err := crypto.GenerateKey()
			require.NoError(t, err)
			keys[i], validators[i] = key, crypto.PubkeyToAddress(key.PublicKey)
		}

		sealed, err := besu.SealHeader(&header, consensus, validators, keys[:3])
		require.NoError(t, err)
		assert.Equal(t, consensus, sealed.Consensus)
		assert.Equal(t, slices.SortedFunc(slices.Values(validators), common.Address.Cmp), sealed.Validators)
		assert.Equal(t, header.Root, sealed.StateRoot)
		assert.Len(t, sealed.CommitSeals, 3)
		assert.Equal(t, sealed.Signers, recoverSeals(t, sealed))

		outsider, err := crypto.GenerateKey()
		require.NoError(t, err)
		_, err = besu.SealHeader(&header, consensus, validators, []*ecdsa.PrivateKey{keys[0], outsider})
		require.ErrorContains(t, err, "is not a validator")
		_, err = besu.SealHeader(&header, consensus, validators, []*ecdsa.PrivateKey{keys[0], keys[1], keys[0]})
		require.ErrorContains(t, err, "repeats")
	}
}

// Validator seals under two consensuses leave the chain's consensus unknown,
// so parsing fails rather than pick one.
func TestParseSealedHeaderRejectsSealsUnderTwoConsensuses(t *testing.T) {
	header := *fixtureHeader(t, besu.QBFT)
	items, _ := splitExtra(t, header.Extra)

	keys := make([]*ecdsa.PrivateKey, len(besu.Consensuses))
	validators := make([]common.Address, len(keys))
	for i := range keys {
		key, err := crypto.GenerateKey()
		require.NoError(t, err)
		keys[i], validators[i] = key, crypto.PubkeyToAddress(key.PublicKey)
	}
	var err error
	items[1], err = rlp.EncodeToBytes(validators)
	require.NoError(t, err)

	seals := make([][]byte, len(keys))
	for i, consensus := range besu.Consensuses {
		digest, digestErr := besu.CommitSealDigest(&header, items, consensus)
		require.NoError(t, digestErr)
		seals[i], err = crypto.Sign(digest, keys[i])
		require.NoError(t, err)
	}
	items[4], err = rlp.EncodeToBytes(seals)
	require.NoError(t, err)
	header.Extra, err = rlp.EncodeToBytes(items)
	require.NoError(t, err)

	_, err = besu.ParseSealedHeader(&header)
	require.ErrorContains(t, err, "commit seals recover to header validators under both")
}

func TestParseSealedHeaderRejectsMalformed(t *testing.T) {
	header := *fixtureHeader(t, besu.QBFT)

	_, err := besu.ParseSealedHeader(nil)
	require.ErrorContains(t, err, "nil header")

	header.Extra = []byte{0x80}
	_, err = besu.ParseSealedHeader(&header)
	require.ErrorContains(t, err, "extra data list")

	header.Extra, err = rlp.EncodeToBytes([][]byte{{}, {}, {}, {}})
	require.NoError(t, err)
	_, err = besu.ParseSealedHeader(&header)
	require.ErrorContains(t, err, "4 extra data items, want 5")
}

// fixtureHeader is the node's header of the fixture's adjacent update.
func fixtureHeader(t *testing.T, consensus besu.Consensus) *types.Header {
	t.Helper()

	update := besutest.MustFixture(t, consensus).AdjacentUpdate

	return besutest.SealedHeader(t, update.HeaderRLP, update.CommitSeals)
}

// recoverSeals recovers header's commit seals over the hash of its RLP, as
// the light client does, requiring each already in the form it accepts.
func recoverSeals(t testing.TB, header *besu.ParsedHeader) []common.Address {
	t.Helper()

	digest := crypto.Keccak256(header.RLP)
	signers := make([]common.Address, len(header.CommitSeals))
	for i, seal := range header.CommitSeals {
		signer, canonical, err := ecrecover.Recover(digest, seal)
		require.NoError(t, err)
		require.Equal(t, seal, canonical, "seal %d not in the light client's form", i)
		signers[i] = signer
	}

	return signers
}

func TestHeightNotAfter(t *testing.T) {
	head := &besu.ParsedHeader{Height: 30, Timestamp: 1030}
	for notAfter, want := range map[uint64]uint64{
		1030: 30, // the head fits
		1040: 30,
		1025: 25, // one block back per second
		1001: 1,
		0:    1, // never genesis
	} {
		assert.Equal(t, want, besu.HeightNotAfter(head, notAfter), "notAfter %d", notAfter)
	}
	assert.Equal(t, uint64(1), besu.HeightNotAfter(&besu.ParsedHeader{Height: 1, Timestamp: 1030}, 0))
}
