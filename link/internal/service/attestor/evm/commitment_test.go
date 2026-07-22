package evm

import (
	"encoding/hex"
	"strings"
	"testing"

	v2 "github.com/cosmos/ibc/link/internal/types/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Solidity fixture:
// https://github.com/cosmos/solidity-ibc-eureka/blob/8a110d3b2463d5703935a3bd170abd95ac42fff6/test/solidity-ibc/ICS24HostTest.t.sol#L49-L71
// Solidity implementation:
// https://github.com/cosmos/solidity-ibc-eureka/blob/8a110d3b2463d5703935a3bd170abd95ac42fff6/contracts/utils/ICS24Host.sol#L103-L140
func TestPacketCommitment(t *testing.T) {
	t.Run("matchesSolidityFixture", func(t *testing.T) {
		// ARRANGE
		value := mustDecodeCommitmentHex(t,
			"0000000000000000000000000000000000000000000000000000000000000020"+
				"00000000000000000000000000000000000000000000000000000000000000a0"+
				"00000000000000000000000000000000000000000000000000000000000000e0"+
				"0000000000000000000000000000000000000000000000000000000000000120"+
				"00000000000000000000000000000000000000000000000000000000000f4240"+
				"0000000000000000000000000000000000000000000000000000000000000160"+
				"0000000000000000000000000000000000000000000000000000000000000005"+
				"7561746f6d000000000000000000000000000000000000000000000000000000"+
				"0000000000000000000000000000000000000000000000000000000000000006"+
				"73656e6465720000000000000000000000000000000000000000000000000000"+
				"0000000000000000000000000000000000000000000000000000000000000008"+
				"7265636569766572000000000000000000000000000000000000000000000000"+
				"0000000000000000000000000000000000000000000000000000000000000004"+
				"6d656d6f00000000000000000000000000000000000000000000000000000000",
		)
		packet := v2.Packet{
			Sequence:         1,
			SourceClient:     "channel-0",
			DestClient:       "channel-1",
			TimeoutTimestamp: 100,
			Payloads: []v2.Payload{
				{
					SourcePort: "transfer",
					DestPort:   "transfer",
					Version:    "ics20-1",
					Encoding:   "application/x-solidity-abi",
					Value:      value,
				},
			},
		}
		expected := mustDecodeCommitmentHex(
			t,
			"0xb691a1950f6fb0bbbcf4bdb16fe2c4d0aa7ef783eb7803073f475cb8164d9b7a",
		)

		// ACT
		actual := PacketCommitment(packet)

		// ASSERT
		assert.Equal(t, expected, actual[:])
	})

	t.Run("hashesEmptyPayloadList", func(t *testing.T) {
		// ARRANGE
		packet := v2.Packet{
			DestClient:       "channel-1",
			TimeoutTimestamp: 100,
		}
		expected := mustDecodeCommitmentHex(
			t,
			"01f5ca8632d9f458e3a915dfb091ad3a477b9eaa582eece6d11cbb8e5dda2d2a",
		)

		// ACT
		actual := PacketCommitment(packet)

		// ASSERT
		assert.Equal(t, expected, actual[:])
	})

	t.Run("preservesPayloadOrder", func(t *testing.T) {
		// ARRANGE
		packet := v2.Packet{
			DestClient:       "channel-1",
			TimeoutTimestamp: 100,
			Payloads: []v2.Payload{
				{SourcePort: "first", Value: []byte("one")},
				{SourcePort: "second", Value: []byte("two")},
			},
		}
		swapped := packet
		swapped.Payloads = []v2.Payload{packet.Payloads[1], packet.Payloads[0]}

		// ACT
		actual := PacketCommitment(packet)
		reordered := PacketCommitment(swapped)

		// ASSERT
		assert.NotEqual(t, actual, reordered)
	})

	t.Run("ignoresPathFields", func(t *testing.T) {
		// ARRANGE
		packet := v2.Packet{
			Sequence:         1,
			SourceClient:     "channel-0",
			DestClient:       "channel-1",
			TimeoutTimestamp: 100,
			Payloads:         []v2.Payload{{Value: []byte("payload")}},
		}
		changedPath := packet
		changedPath.Sequence = 99
		changedPath.SourceClient = "different-source"

		// ACT
		actual := PacketCommitment(packet)
		withChangedPath := PacketCommitment(changedPath)

		// ASSERT
		assert.Equal(t, actual, withChangedPath)
	})
}

func mustDecodeCommitmentHex(t *testing.T, value string) []byte {
	t.Helper()

	decoded, err := hex.DecodeString(strings.TrimPrefix(value, "0x"))
	require.NoError(t, err)

	return decoded
}
