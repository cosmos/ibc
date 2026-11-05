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

func TestSignABIRejectsInvalidSignature(t *testing.T) {
	_, err := SignABI(context.Background(), stubSigner{sig: []byte{0x01}}, TagPacketAttestation, []byte("data"))
	require.Error(t, err)
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
	new(big.Int).Sub(crypto.S256().Params().N, new(big.Int).SetBytes(out[32:64])).FillBytes(out[32:64])
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
