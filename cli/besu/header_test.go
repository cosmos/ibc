// SPDX-License-Identifier: Apache-2.0

package besu_test

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/besu"
	"github.com/cosmos/ibc/cli/besu/besutest"
)

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
