// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func FuzzDecodeStateAttestation(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		height, timestamp, err := DecodeStateAttestation(data)
		if err != nil {
			return
		}

		encoded, err := EncodeStateAttestation(height, timestamp)
		require.NoError(t, err)
		gotHeight, gotTimestamp, err := DecodeStateAttestation(encoded)
		require.NoError(t, err)
		require.Equal(t, height, gotHeight)
		require.Equal(t, timestamp, gotTimestamp)
	})
}

func FuzzDecodePacketAttestation(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		height, packets, err := DecodePacketAttestation(data)
		if err != nil {
			return
		}

		encoded, err := EncodePacketAttestation(height, packets)
		require.NoError(t, err)
		gotHeight, gotPackets, err := DecodePacketAttestation(encoded)
		require.NoError(t, err)
		require.Equal(t, height, gotHeight)
		require.Equal(t, packets, gotPackets)
	})
}

// FuzzRecoverSigner feeds attestor-supplied signatures to RecoverSigner. The
// two forms of a recovery ID, 0/1 and 27/28, must recover the same signer.
func FuzzRecoverSigner(f *testing.F) {
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
