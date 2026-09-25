// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"testing"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

var routerABI = mustRouterABI()

func mustRouterABI() *abi.ABI {
	parsed, err := ics26router.ContractMetaData.GetAbi()
	if err != nil {
		panic(err)
	}
	return parsed
}

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
	clientUpdate := v2.ClientUpdate{ClientID: "ethereum-0", Payloads: [][]byte{{0x01}}}

	callsOf := func(t *testing.T, txs []v2.RelayTx) [][]byte {
		t.Helper()
		require.Len(t, txs, 1)
		requireSelector(t, "multicall", txs[0].Data)

		args, err := routerABI.Methods["multicall"].Inputs.Unpack(txs[0].Data[4:])
		require.NoError(t, err)
		require.Len(t, args, 1)

		calls, ok := args[0].([][]byte)
		require.True(t, ok)

		return calls
	}

	t.Run("recv", func(t *testing.T) {
		items := []v2.PacketRelayItem{
			{Kind: v2.RelayKindRecv, Packet: packet, Proof: []byte{0x02}, ProofHeight: 100},
		}

		txs, err := client.BuildRelayTxs(clientUpdate, items)
		require.NoError(t, err)
		calls := callsOf(t, txs)
		require.Equal(t, router.Bytes(), txs[0].To)
		require.Len(t, calls, 2)
		requireSelector(t, "updateClient", calls[0])
		args, err := routerABI.Methods["updateClient"].Inputs.Unpack(calls[0][4:])
		require.NoError(t, err)
		require.Equal(t, clientUpdate.Payloads[0], args[1])
		requireSelector(t, "recvPacket", calls[1])
	})

	t.Run("several updates in order", func(t *testing.T) {
		items := []v2.PacketRelayItem{
			{Kind: v2.RelayKindRecv, Packet: packet, Proof: []byte{0x02}, ProofHeight: 100},
		}
		update := v2.ClientUpdate{ClientID: clientUpdate.ClientID, Payloads: [][]byte{{0x0a}, {0x0b}}}
		txs, err := client.BuildRelayTxs(update, items)
		require.NoError(t, err)

		calls := callsOf(t, txs)
		require.Len(t, calls, 3)
		for i, payload := range update.Payloads {
			requireSelector(t, "updateClient", calls[i])
			args, err := routerABI.Methods["updateClient"].Inputs.Unpack(calls[i][4:])
			require.NoError(t, err)
			require.Equal(t, payload, args[1])
		}
		requireSelector(t, "recvPacket", calls[2])
	})

	t.Run("no update needed", func(t *testing.T) {
		items := []v2.PacketRelayItem{
			{Kind: v2.RelayKindRecv, Packet: packet, Proof: []byte{0x02}, ProofHeight: 100},
		}
		txs, err := client.BuildRelayTxs(v2.ClientUpdate{ClientID: clientUpdate.ClientID}, items)
		require.NoError(t, err)

		calls := callsOf(t, txs)
		require.Len(t, calls, 1)
		requireSelector(t, "recvPacket", calls[0])
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
