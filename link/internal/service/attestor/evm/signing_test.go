package evm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubSigner struct {
	sig []byte
	err error
}

func (s stubSigner) Sign(context.Context, []byte) ([]byte, error) {
	return s.sig, s.err
}

func TestSignABI(t *testing.T) {
	t.Run("signsDigestAndNormalizesRecoveryID", func(t *testing.T) {
		sig := make([]byte, 65)
		sig[64] = 1 // raw 0/1 recovery id

		signer := stubSigner{sig: sig}

		got, err := SignABI(context.Background(), signer, TagStateAttestation, []byte("data"))
		require.NoError(t, err)
		require.Len(t, got, 65)
		assert.Equal(t, byte(28), got[64], "recovery id must be normalized to Solidity's 27/28 form")
	})

	t.Run("propagatesSignerError", func(t *testing.T) {
		signer := stubSigner{err: assert.AnError}

		_, err := SignABI(context.Background(), signer, TagStateAttestation, []byte("data"))
		require.ErrorIs(t, err, assert.AnError)
	})
}

func TestNormalizeSignature(t *testing.T) {
	for _, tt := range []struct {
		name        string
		recoveryID  byte
		expected    byte
		errContains string
	}{
		{name: "raw0", recoveryID: 0, expected: 27},
		{name: "raw1", recoveryID: 1, expected: 28},
		{name: "legacy27", recoveryID: 27, expected: 27},
		{name: "legacy28", recoveryID: 28, expected: 28},
	} {
		t.Run(tt.name, func(t *testing.T) {
			sig := make([]byte, 65)
			sig[64] = tt.recoveryID

			got, err := normalizeSignature(sig)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, got[64])
		})
	}

	t.Run("rejectsInvalidRecoveryID", func(t *testing.T) {
		sig := make([]byte, 65)
		sig[64] = 99

		_, err := normalizeSignature(sig)
		require.ErrorContains(t, err, "invalid recovery ID")
	})

	t.Run("rejectsWrongLength", func(t *testing.T) {
		_, err := normalizeSignature([]byte{0x01})
		require.ErrorContains(t, err, "invalid signature length")
	})

	t.Run("doesNotMutateInput", func(t *testing.T) {
		sig := make([]byte, 65)
		sig[64] = 0

		_, err := normalizeSignature(sig)
		require.NoError(t, err)
		assert.Equal(t, byte(0), sig[64], "input must not be mutated")
	})
}
