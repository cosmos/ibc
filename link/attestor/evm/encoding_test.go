// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodePacketAttestation(t *testing.T) {
	first := PacketCompact{
		Path:       filledWord(0x11),
		Commitment: filledWord(0x22),
	}
	second := PacketCompact{
		Path:       filledWord(0x33),
		Commitment: filledWord(0x44),
	}

	for _, tt := range []struct {
		name    string
		height  uint64
		packets []PacketCompact
	}{
		{
			name:    "onePacket",
			height:  42,
			packets: []PacketCompact{first},
		},
		{
			name:    "twoPackets",
			height:  ^uint64(0),
			packets: []PacketCompact{first, second},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			expected := expectedPacketAttestation(tt.height, tt.packets)

			// ACT
			actual, err := EncodePacketAttestation(tt.height, tt.packets)

			// ASSERT
			require.NoError(t, err)
			assert.Len(t, actual, 128+64*len(tt.packets))
			assert.Equal(t, expected, actual)

			// decoding must recover the same height and packets
			decodedHeight, decodedPackets, err := DecodePacketAttestation(actual)
			require.NoError(t, err)
			assert.Equal(t, tt.height, decodedHeight)
			assert.Equal(t, tt.packets, decodedPackets)
		})
	}
}

func TestEncodeStateAttestation(t *testing.T) {
	for _, tt := range []struct {
		name      string
		height    uint64
		timestamp uint64
	}{
		{
			name:      "values",
			height:    42,
			timestamp: 1_700_000_000,
		},
		{
			name:      "zero values",
			height:    0,
			timestamp: 0,
		},
		{
			name:      "maximum values",
			height:    ^uint64(0),
			timestamp: ^uint64(0),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// ACT
			encoded, err := EncodeStateAttestation(tt.height, tt.timestamp)

			// ASSERT
			require.NoError(t, err)
			require.Len(t, encoded, 64)
			assert.Equal(t, abiWord(tt.height), encoded[:32])
			assert.Equal(t, abiWord(tt.timestamp), encoded[32:])

			// decoding must recover the same height and timestamp
			decodedHeight, decodedTimestamp, err := DecodeStateAttestation(encoded)
			require.NoError(t, err)
			assert.Equal(t, tt.height, decodedHeight)
			assert.Equal(t, tt.timestamp, decodedTimestamp)
		})
	}
}

func abiWord(value uint64) []byte {
	word := make([]byte, 32)
	binary.BigEndian.PutUint64(word[24:], value)

	return word
}

func filledWord(value byte) [32]byte {
	var word [32]byte
	for i := range word {
		word[i] = value
	}

	return word
}

func expectedPacketAttestation(height uint64, packets []PacketCompact) []byte {
	expected := make([]byte, 0, 128+64*len(packets))
	expected = append(expected, abiWord(32)...)
	expected = append(expected, abiWord(height)...)
	expected = append(expected, abiWord(64)...)
	expected = append(expected, abiWord(uint64(len(packets)))...)
	for _, packet := range packets {
		expected = append(expected, packet.Path[:]...)
		expected = append(expected, packet.Commitment[:]...)
	}

	return expected
}

func TestAttestationProofEncode(t *testing.T) {
	proof := AttestationProof{
		AttestationData: []byte{0x01, 0x02, 0x03},
		Signatures:      [][]byte{make([]byte, 65), make([]byte, 65)},
	}

	encoded, err := EncodeAttestationProof(proof)
	require.NoError(t, err)
	require.NotEmpty(t, encoded)
}
