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
	t.Run("roundTrip", func(t *testing.T) {
		key, err := crypto.GenerateKey()
		require.NoError(t, err)

		expected := crypto.PubkeyToAddress(key.PublicKey)

		digest := Digest(TagPacketAttestation, []byte("attested data"))

		sig, err := crypto.Sign(digest[:], key)
		require.NoError(t, err)
		require.Len(t, sig, 65)

		recovered, err := RecoverSigner(digest, sig)
		require.NoError(t, err)
		require.Equal(t, expected, recovered)
	})

	t.Run("rejectsWrongLength", func(t *testing.T) {
		_, err := RecoverSigner([32]byte{}, []byte{0x01, 0x02})
		require.Error(t, err)
	})

	// AcceptsLegacyVByte verifies RecoverSigner works against signatures
	// using Ethereum's legacy v (27/28), the convention the real attestor
	// signs with (see SignABI/NormalizeSignature), not just the raw 0/1
	// recovery id crypto.Sign itself produces.
	t.Run("acceptsLegacyVByte", func(t *testing.T) {
		key, err := crypto.GenerateKey()
		require.NoError(t, err)

		expected := crypto.PubkeyToAddress(key.PublicKey)

		digest := Digest(TagPacketAttestation, []byte("attested data"))

		sig, err := crypto.Sign(digest[:], key)
		require.NoError(t, err)

		legacySig := make([]byte, 65)
		copy(legacySig, sig)
		legacySig[64] += 27

		recovered, err := RecoverSigner(digest, legacySig)
		require.NoError(t, err)
		require.Equal(t, expected, recovered)
	})

	t.Run("interopsWithSignABI", func(t *testing.T) {
		key, err := crypto.GenerateKey()
		require.NoError(t, err)

		expected := crypto.PubkeyToAddress(key.PublicKey)

		digest := Digest(TagStateAttestation, []byte("data"))

		rawSig, err := crypto.Sign(digest[:], key)
		require.NoError(t, err)

		signer := stubSigner{sig: rawSig}

		sig, err := SignABI(context.Background(), signer, TagStateAttestation, []byte("data"))
		require.NoError(t, err)

		recovered, err := RecoverSigner(digest, sig)
		require.NoError(t, err)
		require.Equal(t, expected, recovered, "RecoverSigner must accept exactly what SignABI produces")
	})

	t.Run("acceptsHighSAsSameSigner", func(t *testing.T) {
		key, digest, sig := signed(t)

		recovered, err := RecoverSigner(digest, malleate(sig))
		require.NoError(t, err)
		require.Equal(t, crypto.PubkeyToAddress(key.PublicKey), recovered)
	})

	// The light client's ecrecover takes only 27/28, so other recovery IDs
	// must not count toward a quorum even when go-ethereum would recover them.
	t.Run("rejectsOtherRecoveryIDs", func(t *testing.T) {
		_, digest, sig := signed(t)

		for _, v := range []byte{2, 3, 26, 29, 30, 31} {
			sig[64] = v
			_, err := RecoverSigner(digest, sig)
			require.ErrorContains(t, err, "invalid recovery ID", "v=%d", v)
		}
	})

	t.Run("rejectsOutOfRangeS", func(t *testing.T) {
		_, digest, sig := signed(t)

		secp256k1N.FillBytes(sig[32:64])
		_, err := RecoverSigner(digest, sig)
		require.ErrorContains(t, err, "out of range")
	})
}

func TestNormalizeSignature(t *testing.T) {
	_, _, sig := signed(t)
	original := append([]byte(nil), sig...)

	for _, form := range [][]byte{sig, malleate(sig)} {
		normalized, err := NormalizeSignature(form)
		require.NoError(t, err)
		require.Equal(t, sig[:64], normalized[:64], "s must be the lower of the pair")
		require.Equal(t, sig[64]+27, normalized[64])
	}
	require.Equal(t, original, sig, "input must not be mutated")
}

func TestSignABINormalizesHighS(t *testing.T) {
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	digest := Digest(TagStateAttestation, []byte("data"))
	rawSig, err := crypto.Sign(digest[:], key)
	require.NoError(t, err)

	sig, err := SignABI(context.Background(), stubSigner{sig: malleate(rawSig)}, TagStateAttestation, []byte("data"))
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
