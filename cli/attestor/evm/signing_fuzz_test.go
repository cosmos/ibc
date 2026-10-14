// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
)

// FuzzRecoverSigner feeds attestor-supplied signatures to RecoverSigner, which
// must count only signatures that normalize to the form the light client
// accepts.
func FuzzRecoverSigner(f *testing.F) {
	key, err := crypto.ToECDSA(common.LeftPadBytes([]byte{1}, 32))
	require.NoError(f, err)
	digest := Digest(TagStateAttestation, []byte("seed"))
	sig, err := crypto.Sign(digest[:], key)
	require.NoError(f, err)
	legacy := append([]byte(nil), sig...)
	legacy[64] += 27
	f.Add(digest[:], sig)
	f.Add(digest[:], legacy)

	f.Fuzz(func(t *testing.T, digest, sig []byte) {
		var d [32]byte
		copy(d[:], digest)

		signer, err := RecoverSigner(d, sig)
		if err != nil {
			return
		}

		normalized, err := NormalizeSignature(sig)
		require.NoError(t, err, "accepted signature does not normalize")
		require.Contains(t, []byte{27, 28}, normalized[64], "normalized v")

		r := new(big.Int).SetBytes(normalized[:32])
		s := new(big.Int).SetBytes(normalized[32:64])
		require.Positive(t, r.Sign(), "r")
		require.Negative(t, r.Cmp(secp256k1N), "r %x not below n", r)
		require.Positive(t, s.Sign(), "s")
		require.LessOrEqual(t, s.Cmp(secp256k1HalfN), 0, "s %x above n/2", s)

		again, err := RecoverSigner(d, normalized)
		require.NoError(t, err, "normalized signature does not recover")
		require.Equal(t, signer, again, "normalized signature recovers a different signer")
	})
}
