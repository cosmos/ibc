// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func FuzzDecodeStateAttestation(f *testing.F) {
	seed, err := EncodeStateAttestation(1, 1_700_000_000)
	require.NoError(f, err)
	f.Add(seed)

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
	seed, err := EncodePacketAttestation(42, []PacketCompact{
		{Path: filledWord(0x11), Commitment: filledWord(0x22)},
		{Path: filledWord(0x33), Commitment: filledWord(0x44)},
	})
	require.NoError(f, err)
	f.Add(seed)

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
