// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
)

// FuzzRecoverSigner feeds attestor-supplied signatures to RecoverSigner. The
// two forms of a recovery ID, 0/1 and 27/28, must recover the same signer.
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

		other := append([]byte(nil), sig...)
		switch other[64] {
		case 0, 1:
			other[64] += 27
		case 27, 28:
			other[64] -= 27
		default:
			return
		}
		otherSigner, err := RecoverSigner(d, other)
		require.NoError(t, err)
		require.Equal(t, signer, otherSigner)
	})
}
