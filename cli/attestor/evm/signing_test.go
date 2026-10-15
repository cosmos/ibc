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
	t.Run("recoversCanonical", func(t *testing.T) {
		key, digest, sig := signed(t)
		sig[64] += 27

		recovered, err := RecoverSigner(digest, sig)
		require.NoError(t, err)
		require.Equal(t, crypto.PubkeyToAddress(key.PublicKey), recovered)
	})

	t.Run("rejectsWrongLength", func(t *testing.T) {
		_, err := RecoverSigner([32]byte{}, []byte{0x01, 0x02})
		require.Error(t, err)
	})

	t.Run("interopsWithSignABI", func(t *testing.T) {
		key, _, rawSig := signed(t)

		sig, err := SignABI(
			context.Background(),
			stubSigner{sig: rawSig},
			TagPacketAttestation,
			[]byte("attested data"),
		)
		require.NoError(t, err)

		recovered, err := RecoverSigner(Digest(TagPacketAttestation, []byte("attested data")), sig)
		require.NoError(t, err)
		require.Equal(
			t,
			crypto.PubkeyToAddress(key.PublicKey),
			recovered,
			"RecoverSigner must accept exactly what SignABI produces",
		)
	})

	// The light client's ecrecover takes only 27/28, so other recovery IDs
	// must not count toward a quorum even when go-ethereum would recover them.
	t.Run("rejectsOtherRecoveryIDs", func(t *testing.T) {
		_, digest, sig := signed(t)

		for _, v := range []byte{0, 1, 2, 3, 26, 29, 30, 31} {
			sig[64] = v
			_, err := RecoverSigner(digest, sig)
			require.ErrorContains(t, err, "invalid recovery ID", "v=%d", v)
		}
	})

	t.Run("rejectsHighS", func(t *testing.T) {
		_, digest, sig := signed(t)
		highS := malleate(sig)
		highS[64] += 27

		_, err := RecoverSigner(digest, highS)
		require.ErrorContains(t, err, "not canonical")
	})

	t.Run("rejectsOutOfRangeS", func(t *testing.T) {
		_, digest, sig := signed(t)
		sig[64] += 27

		secp256k1N.FillBytes(sig[32:64])
		_, err := RecoverSigner(digest, sig)
		require.ErrorContains(t, err, "not canonical")
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
