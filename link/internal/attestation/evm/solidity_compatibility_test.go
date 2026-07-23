package evm

import (
	"encoding/hex"
	"strings"
	"testing"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	hostv2 "github.com/cosmos/ibc-go/v11/modules/core/24-host/v2"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Fixtures from solidity-ibc-eureka commit 8a110d3b2463d5703935a3bd170abd95ac42fff6:
// test/solidity-ibc/ICS24HostTest.t.sol
func TestSolidityCompatibility(t *testing.T) {
	t.Run("packetCommitment", func(t *testing.T) {
		value := mustDecodeHex(t,
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
		packet := channeltypesv2.Packet{
			Sequence:          1,
			SourceClient:      "channel-0",
			DestinationClient: "channel-1",
			TimeoutTimestamp:  100,
			Payloads: []channeltypesv2.Payload{
				{
					SourcePort:      "transfer",
					DestinationPort: "transfer",
					Version:         "ics20-1",
					Encoding:        "application/x-solidity-abi",
					Value:           value,
				},
			},
		}

		actual := channeltypesv2.CommitPacket(packet)
		expected := mustDecodeHex(t, "b691a1950f6fb0bbbcf4bdb16fe2c4d0aa7ef783eb7803073f475cb8164d9b7a")

		assert.Equal(t, expected, actual)
	})

	pathFixtures := []struct {
		name     string
		path     func(string, uint64) []byte
		clientID string
		sequence uint64
		expected string
	}{
		{
			name:     "packetPath",
			path:     hostv2.PacketCommitmentKey,
			clientID: "channel-0",
			sequence: 1,
			expected: "6368616e6e656c2d30010000000000000001",
		},
		{
			name:     "receiptPath",
			path:     hostv2.PacketReceiptKey,
			clientID: "channel-1",
			sequence: 2,
			expected: "6368616e6e656c2d31020000000000000002",
		},
		{
			name:     "acknowledgementPath",
			path:     hostv2.PacketAcknowledgementKey,
			clientID: "channel-2",
			sequence: 3,
			expected: "6368616e6e656c2d32030000000000000003",
		},
	}
	for _, fixture := range pathFixtures {
		t.Run(fixture.name, func(t *testing.T) {
			assert.Equal(t, mustDecodeHex(t, fixture.expected), fixture.path(fixture.clientID, fixture.sequence))
		})
	}

	t.Run("pathHash", func(t *testing.T) {
		path := mustDecodeHex(t, "6368616e6e656c2d30010000000000000001")
		expected := mustDecodeHex(t, "f12abff5cdc0ca904de170332d1278d1002652e8bd9ed9e103cd2ae10d5465a7")

		actual := crypto.Keccak256Hash(path)

		assert.Equal(t, expected, actual[:])
	})
}

func mustDecodeHex(t *testing.T, value string) []byte {
	t.Helper()

	decoded, err := hex.DecodeString(strings.TrimPrefix(value, "0x"))
	require.NoError(t, err)

	return decoded
}
