// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"crypto/sha256"
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
}

type stubSigner struct {
	sig []byte
	err error
}

func (s stubSigner) Sign(context.Context, []byte) ([]byte, error) {
	return s.sig, s.err
}
