// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	"github.com/cosmos/ibc/link/internal/chains/evm/contracts/ics26router"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

func testPacket() ics26router.IICS26RouterMsgsPacket {
	return ics26router.IICS26RouterMsgsPacket{
		Sequence:         1,
		SourceClient:     "base-0",
		DestClient:       "ethereum-0",
		TimeoutTimestamp: 1234567890,
		Payloads: []ics26router.IICS26RouterMsgsPayload{
			{
				SourcePort: "transfer",
				DestPort:   "transfer",
				Version:    "ics20-1",
				Encoding:   "application/x-solidity-abi",
				Value:      []byte{0xde, 0xad},
			},
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
	packed, err := packUpdateClient("ethereum-0", []byte{0x01, 0x02})
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

	call2, err := packUpdateClient("ethereum-0", []byte{0x01})
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

func TestBuildRelayTxs(t *testing.T) {
	router := common.HexToAddress("0x1111111111111111111111111111111111111111")
	client := New(router)

	packet := channeltypesv2.Packet{
		Sequence: 1, SourceClient: "base-0", DestinationClient: "ethereum-0", TimeoutTimestamp: 1234567890,
		Payloads: []channeltypesv2.Payload{
			{
				SourcePort:      "transfer",
				DestinationPort: "transfer",
				Version:         "ics20-1",
				Encoding:        "application/x-solidity-abi",
				Value:           []byte{0xde, 0xad},
			},
		},
	}
	clientUpdate := v2.ClientUpdate{ClientID: "ethereum-0", StateProof: []byte{0x01}}

	t.Run("recv", func(t *testing.T) {
		items := []v2.PacketRelayItem{
			{Kind: v2.RelayKindRecv, Packet: packet, Proof: []byte{0x02}, ProofHeight: 100},
		}

		txs, err := client.BuildRelayTxs(clientUpdate, items)
		require.NoError(t, err)
		require.Len(t, txs, 1)
		require.Equal(t, router.Bytes(), txs[0].To)
		requireSelector(t, "multicall", txs[0].Data)
	})

	t.Run("ackRequiresAckBytes", func(t *testing.T) {
		items := []v2.PacketRelayItem{
			{Kind: v2.RelayKindAck, Packet: packet, Proof: []byte{0x02}, ProofHeight: 100},
		}

		_, err := client.BuildRelayTxs(clientUpdate, items)
		require.Error(t, err)
	})

	t.Run("ack", func(t *testing.T) {
		items := []v2.PacketRelayItem{
			{Kind: v2.RelayKindAck, Packet: packet, Acks: [][]byte{{0xac}}, Proof: []byte{0x02}, ProofHeight: 100},
		}

		txs, err := client.BuildRelayTxs(clientUpdate, items)
		require.NoError(t, err)
		require.Len(t, txs, 1)
	})

	t.Run("timeout", func(t *testing.T) {
		items := []v2.PacketRelayItem{
			{Kind: v2.RelayKindTimeout, Packet: packet, Proof: []byte{0x02}, ProofHeight: 100},
		}

		txs, err := client.BuildRelayTxs(clientUpdate, items)
		require.NoError(t, err)
		require.Len(t, txs, 1)
	})

	t.Run("unsupportedKind", func(t *testing.T) {
		items := []v2.PacketRelayItem{
			{Kind: v2.RelayKindUnknown, Packet: packet},
		}

		_, err := client.BuildRelayTxs(clientUpdate, items)
		require.Error(t, err)
	})

	t.Run("multiPayloadRejected", func(t *testing.T) {
		multiPayloadPacket := packet
		multiPayloadPacket.Payloads = append(multiPayloadPacket.Payloads, multiPayloadPacket.Payloads[0])

		items := []v2.PacketRelayItem{
			{Kind: v2.RelayKindRecv, Packet: multiPayloadPacket, Proof: []byte{0x02}, ProofHeight: 100},
		}

		_, err := client.BuildRelayTxs(clientUpdate, items)
		require.ErrorContains(t, err, "only supports single-payload packets")
	})
}
