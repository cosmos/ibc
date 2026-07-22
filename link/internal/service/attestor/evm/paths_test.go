package evm

import (
	"encoding/hex"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Solidity fixtures:
// https://github.com/cosmos/solidity-ibc-eureka/blob/8a110d3b2463d5703935a3bd170abd95ac42fff6/test/solidity-ibc/ICS24HostTest.t.sol#L85-L101
// Solidity implementation:
// https://github.com/cosmos/solidity-ibc-eureka/blob/8a110d3b2463d5703935a3bd170abd95ac42fff6/contracts/utils/ICS24Host.sol#L21-L100
func TestCommitmentPaths(t *testing.T) {

	for _, tt := range []struct {
		name     string
		path     func(string, uint64) []byte
		clientID string
		sequence uint64
		expected string
	}{
		{
			name:     "packetPath",
			path:     PathPacket,
			clientID: "channel-0",
			sequence: 1,
			expected: "6368616e6e656c2d30010000000000000001",
		},
		{
			name:     "receiptPath",
			path:     PathReceipt,
			clientID: "channel-1",
			sequence: 2,
			expected: "6368616e6e656c2d31020000000000000002",
		},
		{
			name:     "ackPath",
			path:     PathAck,
			clientID: "channel-2",
			sequence: 3,
			expected: "6368616e6e656c2d32030000000000000003",
		},
		{
			name:     "maximumSequence",
			path:     PathPacket,
			clientID: "channel-0",
			sequence: math.MaxUint64,
			expected: "6368616e6e656c2d3001ffffffffffffffff",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			expected := mustDecodePathHex(t, tt.expected)

			// ACT
			actual := tt.path(tt.clientID, tt.sequence)

			// ASSERT
			assert.Equal(t, expected, actual)
		})
	}

	t.Run("hashMatchesSolidity", func(t *testing.T) {
		// ARRANGE
		path := mustDecodePathHex(t, "6368616e6e656c2d30010000000000000001")
		expected := mustDecodePathHex(
			t,
			"f12abff5cdc0ca904de170332d1278d1002652e8bd9ed9e103cd2ae10d5465a7",
		)

		// ACT
		actual := PathHash(path)

		// ASSERT
		assert.Equal(t, expected, actual[:])
	})
}

func mustDecodePathHex(t *testing.T, value string) []byte {
	t.Helper()

	decoded, err := hex.DecodeString(value)
	require.NoError(t, err)

	return decoded
}
