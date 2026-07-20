package attestation

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/chains/evm/contracts/ics26router"
)

func testPacket() ics26router.IICS26RouterMsgsPacket {
	return ics26router.IICS26RouterMsgsPacket{
		Sequence:         1,
		SourceClient:     "base-0",
		DestClient:       "ethereum-0",
		TimeoutTimestamp: 1234567890,
		Payloads: []ics26router.IICS26RouterMsgsPayload{
			{SourcePort: "transfer", DestPort: "transfer", Version: "ics20-1", Encoding: "application/x-solidity-abi", Value: []byte{0xde, 0xad}},
		},
	}
}

func requireSelector(t *testing.T, method string, packed []byte) {
	t.Helper()

	require.NotEmpty(t, packed)
	require.GreaterOrEqual(t, len(packed), 4)
	require.Equal(t, routerABI.Methods[method].ID, packed[:4])
}

func TestPackUpdateClient(t *testing.T) {
	proof := AttestationProof{AttestationData: []byte{0x01, 0x02}, Signatures: [][]byte{make([]byte, 65)}}

	packed, err := packUpdateClient("ethereum-0", proof)
	require.NoError(t, err)
	requireSelector(t, "updateClient", packed)
}

func TestPackRecvPacket(t *testing.T) {
	packed, err := packRecvPacket(testPacket(), []byte{0x01, 0x02}, 100)
	require.NoError(t, err)
	requireSelector(t, "recvPacket", packed)
}

func TestPackAckPacket(t *testing.T) {
	packed, err := packAckPacket(testPacket(), []byte{0x01}, []byte{0x02}, 100)
	require.NoError(t, err)
	requireSelector(t, "ackPacket", packed)
}

func TestPackTimeoutPacket(t *testing.T) {
	packed, err := packTimeoutPacket(testPacket(), []byte{0x01, 0x02}, 100)
	require.NoError(t, err)
	requireSelector(t, "timeoutPacket", packed)
}

func TestPackMulticall(t *testing.T) {
	call1, err := packRecvPacket(testPacket(), []byte{0x01}, 100)
	require.NoError(t, err)

	proof := AttestationProof{AttestationData: []byte{0x01}, Signatures: [][]byte{make([]byte, 65)}}
	call2, err := packUpdateClient("ethereum-0", proof)
	require.NoError(t, err)

	packed, err := packMulticall([][]byte{call2, call1})
	require.NoError(t, err)
	requireSelector(t, "multicall", packed)

	args, err := routerABI.Methods["multicall"].Inputs.Unpack(packed[4:])
	require.NoError(t, err)
	require.Len(t, args, 1)

	calls, ok := args[0].([][]byte)
	require.True(t, ok)
	require.Equal(t, [][]byte{call2, call1}, calls)
}
