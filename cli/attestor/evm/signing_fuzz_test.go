// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
)

// FuzzRecoverSigner checks RecoverSigner against go-ethereum: it must recover
// exactly the signatures go-ethereum does with v of 0/1 or 27/28, to the same
// signer, and return a form of each that the light client accepts.
func FuzzRecoverSigner(f *testing.F) {
	key, err := crypto.ToECDSA(common.LeftPadBytes([]byte{1}, 32))
	require.NoError(f, err)
	digest := Digest(TagStateAttestation, []byte("seed"))
	sig, err := crypto.Sign(digest[:], key)
	require.NoError(f, err)
	for _, form := range [][]byte{sig, malleate(sig)} {
		legacy := append([]byte(nil), form...)
		legacy[64] += 27
		f.Add(digest[:], form)
		f.Add(digest[:], legacy)
	}

	f.Fuzz(func(t *testing.T, digest, sig []byte) {
		var d [32]byte
		copy(d[:], digest)

		signer, canonical, err := RecoverSigner(d, sig)
		want, wantErr := gethSigner(d, sig)
		if wantErr != nil {
			require.Error(t, err, "recovered a signature go-ethereum does not")
			return
		}
		require.NoError(t, err, "rejected a signature go-ethereum recovers")
		require.Equal(t, want, signer)

		// What the light client's ECDSA.recover accepts.
		require.Contains(t, []byte{27, 28}, canonical[64])
		r, s := new(big.Int).SetBytes(canonical[:32]), new(big.Int).SetBytes(canonical[32:64])
		require.True(t, crypto.ValidateSignatureValues(canonical[64]-27, r, s, true), "s above n/2")
		got, err := gethSigner(d, canonical)
		require.NoError(t, err)
		require.Equal(t, signer, got, "returned signature recovers a different signer")
	})
}

// gethSigner is go-ethereum's recovery of sig with v of 0/1 or 27/28.
func gethSigner(digest [32]byte, sig []byte) (common.Address, error) {
	if len(sig) != 65 {
		return common.Address{}, errors.New("invalid length")
	}
	raw := append([]byte(nil), sig...)
	switch raw[64] {
	case 0, 1:
	case 27, 28:
		raw[64] -= 27
	default:
		return common.Address{}, errors.New("invalid recovery ID")
	}

	pub, err := crypto.SigToPub(digest[:], raw)
	if err != nil {
		return common.Address{}, err
	}

	return crypto.PubkeyToAddress(*pub), nil
}
