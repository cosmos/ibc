// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDigestMatchesFormula(t *testing.T) {
	data := []byte("some attestation data")

	for _, tag := range []byte{TagStateAttestation, TagPacketAttestation} {
		got := Digest(tag, data)

		inner := sha256.Sum256(data)
		want := sha256.Sum256(append([]byte{tag}, inner[:]...))

		assert.Equal(t, want, got, "tag %x", tag)
	}
}

func TestRecoverSigner(t *testing.T) {
	// The light client's ECDSA.recover takes only v of 27/28 and low s, but
	// attestors may return any form of the same signature.
	t.Run("acceptsEveryFormAndReturnsCanonical", func(t *testing.T) {
		key, digest, sig := signed(t)
		canonical := append([]byte(nil), sig...)
		canonical[64] += 27

		for _, form := range [][]byte{sig, malleate(sig)} {
			for _, v := range []byte{form[64], form[64] + 27} {
				input := append([]byte(nil), form...)
				input[64] = v
				original := append([]byte(nil), input...)

				recovered, normalized, err := RecoverSigner(digest, input)
				require.NoError(t, err)
				require.Equal(t, crypto.PubkeyToAddress(key.PublicKey), recovered)
				require.Equal(t, canonical, normalized)
				require.Equal(t, original, input, "input must not be mutated")
			}
		}
	})

	t.Run("rejectsWrongLength", func(t *testing.T) {
		_, _, err := RecoverSigner([32]byte{}, []byte{0x01, 0x02})
		require.Error(t, err)
	})

	t.Run("rejectsOtherRecoveryIDs", func(t *testing.T) {
		_, digest, sig := signed(t)

		for _, v := range []byte{2, 3, 26, 29, 30, 31} {
			sig[64] = v
			_, _, err := RecoverSigner(digest, sig)
			require.ErrorContains(t, err, "invalid recovery ID", "v=%d", v)
		}
	})

	t.Run("rejectsOutOfRangeS", func(t *testing.T) {
		_, digest, sig := signed(t)

		secp256k1N.FillBytes(sig[32:64])
		_, _, err := RecoverSigner(digest, sig)
		require.Error(t, err)
	})

	t.Run("returnsSignABIOutputUnchanged", func(t *testing.T) {
		key, _, rawSig := signed(t)

		sig, err := SignABI(
			context.Background(),
			stubSigner{sig: rawSig},
			TagPacketAttestation,
			[]byte("attested data"),
		)
		require.NoError(t, err)

		recovered, normalized, err := RecoverSigner(Digest(TagPacketAttestation, []byte("attested data")), sig)
		require.NoError(t, err)
		require.Equal(t, crypto.PubkeyToAddress(key.PublicKey), recovered)
		require.Equal(t, sig, normalized)
	})
}

func TestSignABINormalizesHighS(t *testing.T) {
	_, _, rawSig := signed(t)

	sig, err := SignABI(
		context.Background(),
		stubSigner{sig: malleate(rawSig)},
		TagPacketAttestation,
		[]byte("attested data"),
	)
	require.NoError(t, err)
	require.Equal(t, rawSig[:64], sig[:64])
	require.Equal(t, rawSig[64]+27, sig[64])
}

// signed returns a fresh key, a digest and the key's raw 0/1 signature over it.
func signed(t *testing.T) (*ecdsa.PrivateKey, [32]byte, []byte) {
	t.Helper()

	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	digest := Digest(TagPacketAttestation, []byte("attested data"))
	sig, err := crypto.Sign(digest[:], key)
	require.NoError(t, err)

	return key, digest, sig
}

// malleate returns the high-s twin of a raw 0/1 low-s signature, which
// recovers to the same signer.
func malleate(sig []byte) []byte {
	out := append([]byte(nil), sig...)
	new(big.Int).Sub(secp256k1N, new(big.Int).SetBytes(out[32:64])).FillBytes(out[32:64])
	out[64] ^= 1

	return out
}

type stubSigner struct {
	sig []byte
	err error
}

func (s stubSigner) Sign(context.Context, []byte) ([]byte, error) {
	return s.sig, s.err
}
