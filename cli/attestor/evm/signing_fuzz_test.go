// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
)

// FuzzRecoverSigner checks NormalizeSignature against go-ethereum: the
// normalized form of an attestor-supplied signature must recover exactly when
// the original does, to the same signer, and RecoverSigner must accept only
// signatures already in that form.
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

		if _, err := RecoverSigner(d, sig); err == nil {
			normalized, normErr := NormalizeSignature(sig)
			require.NoError(t, normErr)
			require.Equal(t, sig, normalized, "accepted a signature that is not canonical")
		}

		normalized, err := NormalizeSignature(sig)
		if err != nil {
			return
		}
		raw := append([]byte(nil), sig...)
		if raw[64] >= 27 {
			raw[64] -= 27
		}
		want, wantErr := crypto.SigToPub(d[:], raw)
		got, err := RecoverSigner(d, normalized)
		if wantErr != nil {
			require.Error(t, err, "normalized form recovers but the original does not")
			return
		}
		require.NoError(t, err, "original recovers but the normalized form does not")
		require.Equal(t, crypto.PubkeyToAddress(*want), got)
	})
}
