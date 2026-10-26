// SPDX-License-Identifier: Apache-2.0

package ecrecover_test

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/internal/pkg/ecrecover"
)

func TestRecover(t *testing.T) {
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	digest := crypto.Keccak256([]byte("signed"))
	sig, err := crypto.Sign(digest, key)
	require.NoError(t, err)

	t.Run("acceptsEveryFormAndReturnsCanonical", func(t *testing.T) {
		for _, form := range [][]byte{sig, malleate(sig)} {
			for _, v := range []byte{form[64], form[64] + 27} {
				input := append([]byte(nil), form...)
				input[64] = v
				original := append([]byte(nil), input...)

				signer, canonical, err := ecrecover.Recover(digest, input)
				require.NoError(t, err)
				require.Equal(t, crypto.PubkeyToAddress(key.PublicKey), signer)
				require.Equal(t, sig, canonical)
				require.Equal(t, original, input, "input must not be mutated")
			}
		}
	})

	t.Run("rejectsWrongLength", func(t *testing.T) {
		_, _, err := ecrecover.Recover(digest, sig[:64])
		require.ErrorContains(t, err, "invalid signature length")
	})

	t.Run("rejectsOtherRecoveryIDs", func(t *testing.T) {
		for _, v := range []byte{2, 3, 4, 26, 29, 30, 31} {
			input := append([]byte(nil), sig...)
			input[64] = v
			_, _, err := ecrecover.Recover(digest, input)
			require.ErrorContains(t, err, "invalid recovery ID", "v=%d", v)
		}
	})

	t.Run("rejectsOutOfRangeS", func(t *testing.T) {
		input := append([]byte(nil), sig...)
		crypto.S256().Params().N.FillBytes(input[32:64])
		_, _, err := ecrecover.Recover(digest, input)
		require.ErrorContains(t, err, "curve order")
	})
}

// FuzzRecover checks Recover against go-ethereum: it must recover exactly the
// signatures go-ethereum does with v of 0/1 or 27/28, to the same signer, and
// return a canonical form of each.
func FuzzRecover(f *testing.F) {
	key, err := crypto.ToECDSA(common.LeftPadBytes([]byte{1}, 32))
	require.NoError(f, err)
	digest := crypto.Keccak256([]byte("seed"))
	sig, err := crypto.Sign(digest, key)
	require.NoError(f, err)
	for _, form := range [][]byte{sig, malleate(sig)} {
		legacy := append([]byte(nil), form...)
		legacy[64] += 27
		f.Add(digest, form)
		f.Add(digest, legacy)
	}

	f.Fuzz(func(t *testing.T, digest, sig []byte) {
		signer, canonical, err := ecrecover.Recover(digest, sig)
		want, wantErr := gethSigner(digest, sig)
		if wantErr != nil {
			require.Error(t, err, "recovered a signature go-ethereum does not")
			return
		}
		require.NoError(t, err, "rejected a signature go-ethereum recovers")
		require.Equal(t, want, signer)

		r, s := new(big.Int).SetBytes(canonical[:32]), new(big.Int).SetBytes(canonical[32:64])
		require.True(t, crypto.ValidateSignatureValues(canonical[64], r, s, true), "not canonical")
		got, err := gethSigner(digest, canonical)
		require.NoError(t, err)
		require.Equal(t, signer, got, "canonical form recovers a different signer")
	})
}

// gethSigner is go-ethereum's recovery of sig with v of 0/1 or 27/28.
func gethSigner(digest, sig []byte) (common.Address, error) {
	if len(sig) != crypto.SignatureLength {
		return common.Address{}, errInvalid
	}
	raw := append([]byte(nil), sig...)
	switch raw[64] {
	case 0, 1:
	case 27, 28:
		raw[64] -= 27
	default:
		return common.Address{}, errInvalid
	}

	pub, err := crypto.SigToPub(digest, raw)
	if err != nil {
		return common.Address{}, err
	}

	return crypto.PubkeyToAddress(*pub), nil
}

var errInvalid = errors.New("invalid signature")

// malleate returns the high-s twin of a raw 0/1 low-s signature, which
// recovers to the same signer.
func malleate(sig []byte) []byte {
	out := append([]byte(nil), sig...)
	new(big.Int).Sub(crypto.S256().Params().N, new(big.Int).SetBytes(out[32:64])).FillBytes(out[32:64])
	out[64] ^= 1

	return out
}
