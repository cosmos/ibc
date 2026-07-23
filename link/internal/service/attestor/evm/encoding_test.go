package evm

import (
	"encoding/binary"
	"testing"

	"github.com/cosmos/ibc/link/internal/chains/evm/contracts/ics26router"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodePacket(t *testing.T) {
	t.Run("decodesPacketAndPayloads", func(t *testing.T) {
		// ARRANGE
		expected := v2.Packet{
			Sequence:         7,
			SourceClient:     "source-client",
			DestClient:       "destination-client",
			TimeoutTimestamp: 1_700_000_000,
			Payloads: []v2.Payload{
				{
					SourcePort: "transfer",
					DestPort:   "transfer",
					Version:    "ics20-1",
					Encoding:   "application/json",
					Value:      []byte("payload"),
				},
				{
					SourcePort: "gmp",
					DestPort:   "destination-gmp",
					Version:    "ics27-1",
					Encoding:   "application/x-solidity-abi",
					Value:      []byte{1, 2, 3},
				},
			},
		}
		contractABI, err := ics26router.ContractMetaData.GetAbi()
		require.NoError(t, err)
		encoded, err := contractABI.Methods["isPacketReceived"].Inputs.Pack(
			ics26router.IICS26RouterMsgsPacket{
				Sequence:         expected.Sequence,
				SourceClient:     expected.SourceClient,
				DestClient:       expected.DestClient,
				TimeoutTimestamp: expected.TimeoutTimestamp,
				Payloads: []ics26router.IICS26RouterMsgsPayload{
					{
						SourcePort: expected.Payloads[0].SourcePort,
						DestPort:   expected.Payloads[0].DestPort,
						Version:    expected.Payloads[0].Version,
						Encoding:   expected.Payloads[0].Encoding,
						Value:      expected.Payloads[0].Value,
					},
					{
						SourcePort: expected.Payloads[1].SourcePort,
						DestPort:   expected.Payloads[1].DestPort,
						Version:    expected.Payloads[1].Version,
						Encoding:   expected.Payloads[1].Encoding,
						Value:      expected.Payloads[1].Value,
					},
				},
			},
		)
		require.NoError(t, err)

		// ACT
		actual, err := DecodePacket(encoded)

		// ASSERT
		require.NoError(t, err)
		assert.Equal(t, expected, actual)
	})

	t.Run("rejectsMalformedABI", func(t *testing.T) {
		// ACT
		packet, err := DecodePacket([]byte{1, 2, 3})

		// ASSERT
		require.ErrorContains(t, err, "unpack packet")
		assert.Empty(t, packet)
	})
}

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
