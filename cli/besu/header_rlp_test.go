// SPDX-License-Identifier: Apache-2.0

package besu_test

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/besu"
	"github.com/cosmos/ibc/cli/besu/besutest"
)

func TestParseHeaderFixture(t *testing.T) {
	fixture := besutest.MustFixture(t)

	for _, update := range []besutest.UpdateFixture{fixture.AdjacentUpdate, fixture.NonAdjacentUpdate} {
		header, err := besu.ParseHeader(update.HeaderRLP)
		require.NoError(t, err)

		assert.Equal(t, update.Height, header.Height)
		assert.Equal(t, update.ExpectedTimestamp, header.Timestamp)
		assert.Equal(t, update.ExpectedValidators, header.Validators)
		assert.Equal(t, []byte(update.HeaderRLP), header.RLP)

		// QBFT blocks carry the committing quorum, not every validator's seal.
		signers, err := header.Signers()
		require.NoError(t, err)
		assert.Subset(t, update.ExpectedValidators, signers, "real seals recover to validators")
		assert.GreaterOrEqual(t, len(signers), besu.QuorumRequired(len(update.ExpectedValidators)))

		require.NoError(t, besu.CheckUpdate(header, signers, fixture.InitialConsensusState()))
	}
}

func TestParseHeaderRejectsMalformed(t *testing.T) {
	fixture := besutest.MustFixture(t)
	template := fixture.AdjacentUpdate.HeaderRLP

	t.Run("not a list", func(t *testing.T) {
		_, err := besu.ParseHeader([]byte{0x80})
		require.ErrorIs(t, err, besu.ErrInvalidHeader)
	})

	t.Run("invalid state root", func(t *testing.T) {
		var items []rlp.RawValue
		require.NoError(t, rlp.DecodeBytes(template, &items))
		for _, root := range []any{make([]byte, 31), make([]byte, 33), []any{}} {
			var err error
			items[3], err = rlp.EncodeToBytes(root)
			require.NoError(t, err)
			raw, err := rlp.EncodeToBytes(items)
			require.NoError(t, err)
			_, err = besu.ParseHeader(raw)
			require.ErrorIs(t, err, besu.ErrInvalidHeader)
			require.ErrorContains(t, err, "state root")
		}
	})

	t.Run("zero validator", func(t *testing.T) {
		validators := append([]common.Address{{}}, fixture.InitialTrustedValidators[1:]...)
		_, err := besu.ParseHeader(besutest.MustBuilder(template).SetValidators(validators).MustEncode())
		require.ErrorIs(t, err, besu.ErrInvalidHeader)
		assert.ErrorContains(t, err, "zero validator")
	})

	t.Run("duplicate validator", func(t *testing.T) {
		v := fixture.InitialTrustedValidators
		_, err := besu.ParseHeader(
			besutest.MustBuilder(template).SetValidators([]common.Address{v[0], v[0], v[1]}).MustEncode(),
		)
		require.ErrorIs(t, err, besu.ErrInvalidHeader)
		assert.ErrorContains(t, err, "duplicate validator")
	})

	t.Run("empty validator set", func(t *testing.T) {
		_, err := besu.ParseHeader(besutest.MustBuilder(template).SetValidators(nil).MustEncode())
		require.ErrorIs(t, err, besu.ErrInvalidHeader)
	})
}

func TestCommitSealDigestIgnoresSeals(t *testing.T) {
	fixture := besutest.MustFixture(t)
	builder := besutest.MustBuilder(fixture.AdjacentUpdate.HeaderRLP)

	original, err := builder.Header()
	require.NoError(t, err)

	stripped, err := builder.SetCommitSeals(nil).Header()
	require.NoError(t, err)

	d1, err := original.CommitSealDigest()
	require.NoError(t, err)
	d2, err := stripped.CommitSealDigest()
	require.NoError(t, err)
	assert.Equal(t, d1, d2)
}

func TestEncodeHeaderRejectsNil(t *testing.T) {
	_, err := besu.EncodeHeader(nil)
	require.Error(t, err)
}

func TestParseSealedHeader(t *testing.T) {
	fixture := besutest.MustFixture(t)
	update := fixture.AdjacentUpdate
	var header types.Header
	require.NoError(t, rlp.DecodeBytes(update.HeaderRLP, &header))

	got, err := besu.ParseSealedHeader(&header)
	require.NoError(t, err)
	assert.Equal(t, []byte(update.HeaderRLP), got.RLP)
	assert.Equal(t, update.Height, got.Height)
	assert.Equal(t, update.ExpectedTimestamp, got.Timestamp)
	assert.Equal(t, update.ExpectedStateRoot, got.StateRoot)
	assert.Equal(t, update.ExpectedValidators, got.Validators)

	_, err = besu.ParseSealedHeader(nil)
	require.Error(t, err)
}

func TestHeaderRLPPreservesTrailingFields(t *testing.T) {
	template := besutest.MustFixture(t).AdjacentUpdate.HeaderRLP
	var items []rlp.RawValue
	require.NoError(t, rlp.DecodeBytes(template, &items))
	// A future fork's fields must survive parsing, mutation and digest encoding.
	tail, err := rlp.EncodeToBytes([]byte("unknown fork field"))
	require.NoError(t, err)
	items = append(items, tail)
	raw, err := rlp.EncodeToBytes(items)
	require.NoError(t, err)
	builder := besutest.MustBuilder(raw)
	assert.Equal(t, raw, builder.MustEncode())
	header, err := builder.Header()
	require.NoError(t, err)
	assert.Equal(t, raw, header.RLP)
	digest, err := header.CommitSealDigest()
	require.NoError(t, err)
	original, err := besu.ParseHeader(template)
	require.NoError(t, err)
	originalDigest, err := original.CommitSealDigest()
	require.NoError(t, err)
	assert.NotEqual(t, originalDigest, digest, "digest must include trailing fields")
	stripped := builder.SetCommitSeals(nil).MustEncode()
	assert.Equal(t, crypto.Keccak256Hash(stripped), digest)
}

func TestParseHeaderRejectsInvalidBFTFields(t *testing.T) {
	template := besutest.MustFixture(t).AdjacentUpdate.HeaderRLP
	for _, tc := range []struct {
		name  string
		index int
		value any
	}{
		{"ommers hash", 1, common.Hash{}},
		{"difficulty", 7, uint64(0)},
		{"mix hash", 13, common.Hash{}},
		{"nonce", 14, []byte{0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var items []rlp.RawValue
			require.NoError(t, rlp.DecodeBytes(template, &items))
			var err error
			items[tc.index], err = rlp.EncodeToBytes(tc.value)
			require.NoError(t, err)
			raw, err := rlp.EncodeToBytes(items)
			require.NoError(t, err)
			_, err = besu.ParseHeader(raw)
			require.ErrorIs(t, err, besu.ErrInvalidHeader)
			require.ErrorContains(t, err, tc.name)
		})
	}
}
