// SPDX-License-Identifier: Apache-2.0

package besu_test

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
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

func TestValidateValidatorsRequiresSortedSet(t *testing.T) {
	v := besutest.MustFixture(t).InitialTrustedValidators
	require.NoError(t, besu.ValidateValidators(v))
	require.ErrorContains(t, besu.ValidateValidators([]common.Address{v[1], v[0]}), "out of order")
}

func TestSignersRejectsBadSeals(t *testing.T) {
	fixture := besutest.MustFixture(t)
	keys := besutest.Keys(4)
	builder := besutest.MustBuilder(fixture.AdjacentUpdate.HeaderRLP).SetValidators(besutest.Addresses(keys))

	header, err := builder.MustSign(keys...).Header()
	require.NoError(t, err)

	signers, err := header.Signers()
	require.NoError(t, err)
	assert.Equal(t, besutest.Addresses(keys), signers)

	t.Run("v as 27/28 is accepted", func(t *testing.T) {
		seals := make([][]byte, len(header.CommitSeals))
		for i, seal := range header.CommitSeals {
			seals[i] = append([]byte(nil), seal...)
			seals[i][64] += 27
		}

		shifted, err := builder.SetCommitSeals(seals).Header()
		require.NoError(t, err)

		got, err := shifted.Signers()
		require.NoError(t, err)
		assert.Equal(t, besutest.Addresses(keys), got)
	})

	t.Run("duplicate signer", func(t *testing.T) {
		dup, err := builder.MustSign(keys[0], keys[0], keys[1]).Header()
		require.NoError(t, err)

		_, err = dup.Signers()
		require.ErrorIs(t, err, besu.ErrDuplicateSigner)
	})

	t.Run("wrong length", func(t *testing.T) {
		short, err := builder.SetCommitSeals([][]byte{make([]byte, 64)}).Header()
		require.NoError(t, err)

		_, err = short.Signers()
		require.ErrorIs(t, err, besu.ErrInvalidSeal)
	})

	t.Run("high s", func(t *testing.T) {
		digest, err := header.CommitSealDigest()
		require.NoError(t, err)

		seal, err := crypto.Sign(digest.Bytes(), keys[0])
		require.NoError(t, err)

		// s' = N - s flips the recovery id and lands in the upper half.
		s := new(big.Int).SetBytes(seal[32:64])
		s.Sub(crypto.S256().Params().N, s)
		s.FillBytes(seal[32:64])
		seal[64] ^= 1

		_, err = besu.RecoverSealSigner(digest, seal)
		require.ErrorIs(t, err, besu.ErrInvalidSeal)
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
