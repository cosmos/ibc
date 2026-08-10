package ibc

import (
	"testing"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
)

func TestDecodePacket(t *testing.T) {
	t.Run("decodesPacketAndPayloads", func(t *testing.T) {
		expected := channeltypesv2.Packet{
			Sequence:          7,
			SourceClient:      "source-client",
			DestinationClient: "destination-client",
			TimeoutTimestamp:  1_700_000_000,
			Payloads: []channeltypesv2.Payload{
				{
					SourcePort:      "transfer",
					DestinationPort: "transfer",
					Version:         "ics20-1",
					Encoding:        "application/json",
					Value:           []byte("payload"),
				},
				{
					SourcePort:      "gmp",
					DestinationPort: "destination-gmp",
					Version:         "ics27-1",
					Encoding:        "application/x-solidity-abi",
					Value:           []byte{1, 2, 3},
				},
			},
		}
		contractABI, err := ics26router.ContractMetaData.GetAbi()
		require.NoError(t, err)
		encoded, err := contractABI.Events["SendPacket"].Inputs.NonIndexed().Pack(
			ics26router.IICS26RouterMsgsPacket{
				Sequence:         expected.Sequence,
				SourceClient:     expected.SourceClient,
				DestClient:       expected.DestinationClient,
				TimeoutTimestamp: expected.TimeoutTimestamp,
				Payloads: []ics26router.IICS26RouterMsgsPayload{
					{
						SourcePort: expected.Payloads[0].SourcePort,
						DestPort:   expected.Payloads[0].DestinationPort,
						Version:    expected.Payloads[0].Version,
						Encoding:   expected.Payloads[0].Encoding,
						Value:      expected.Payloads[0].Value,
					},
					{
						SourcePort: expected.Payloads[1].SourcePort,
						DestPort:   expected.Payloads[1].DestinationPort,
						Version:    expected.Payloads[1].Version,
						Encoding:   expected.Payloads[1].Encoding,
						Value:      expected.Payloads[1].Value,
					},
				},
			},
		)
		require.NoError(t, err)

		actual, err := DecodePacket(encoded)
		require.NoError(t, err)
		assert.Equal(t, expected, actual)
	})

	t.Run("rejectsMalformedABI", func(t *testing.T) {
		packet, err := DecodePacket([]byte{1, 2, 3})
		require.ErrorContains(t, err, "unpack packet")
		assert.Empty(t, packet)
	})

	t.Run("roundTripsThroughEncodePacket", func(t *testing.T) {
		expected := channeltypesv2.Packet{
			Sequence:          7,
			SourceClient:      "source-client",
			DestinationClient: "destination-client",
			TimeoutTimestamp:  1_700_000_000,
			Payloads: []channeltypesv2.Payload{{
				SourcePort:      "transfer",
				DestinationPort: "transfer",
				Version:         "ics20-1",
				Encoding:        "application/json",
				Value:           []byte("payload"),
			}},
		}

		encoded, err := EncodePacket(expected)
		require.NoError(t, err)
		actual, err := DecodePacket(encoded)
		require.NoError(t, err)
		assert.Equal(t, expected, actual)
	})
}
