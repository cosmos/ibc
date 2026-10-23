// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc-contracts/packages/go-abigen/ibcmulticall"
	"github.com/cosmos/ibc-contracts/packages/go-abigen/ics26router"
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

func TestBuildRelayTxs(t *testing.T) {
	router, multicall := common.HexToAddress("0x1111"), common.HexToAddress("0x2222")
	builder := New(router, multicall)
	items := relayItems(3)
	items[1].Kind, items[1].Acks = v2.RelayKindAck, [][]byte{{2}}
	items[2].Kind = v2.RelayKindTimeout
	for _, tc := range []struct {
		name    string
		updates [][]byte
	}{
		{"updates in order", [][]byte{{5}, {6}}},
		{"no update needed", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			update := v2.ClientUpdate{ClientID: "client-0", Payloads: tc.updates}
			txs, err := builder.BuildRelayTxs(update, items)
			require.NoError(t, err)
			require.Len(t, txs, 1)
			require.Equal(t, multicall.Bytes(), txs[0].To)
			calls := decodeMulticall(t, txs[0].Data)
			require.Len(t, calls, len(tc.updates)+len(items))
			methods := []string{"recvPacket", "ackPacket", "timeoutPacket"}
			for i, call := range calls {
				require.Equal(t, router, call.Target)
				if i < len(tc.updates) {
					require.False(t, call.AllowFailure)
					requireSelector(t, "updateClient", call.CallData)
					args, err := routerABI.Methods["updateClient"].Inputs.Unpack(call.CallData[4:])
					require.NoError(t, err)
					require.Equal(t, []any{update.ClientID, tc.updates[i]}, args)
				} else {
					require.True(t, call.AllowFailure)
					requireSelector(t, methods[i-len(tc.updates)], call.CallData)
				}
			}
		})
	}
}

func TestBuildRelayTxsInvalidInput(t *testing.T) {
	router, multicall := common.HexToAddress("0x1111"), common.HexToAddress("0x2222")
	for _, tc := range []struct {
		name     string
		kind     v2.RelayKind
		payloads int
		want     string
	}{
		{"missing acknowledgement", v2.RelayKindAck, 1, "no acknowledgement"},
		{"unsupported kind", v2.RelayKindUnknown, 1, "unsupported relay kind"},
		{"multiple payloads", v2.RelayKindRecv, 2, "only supports single-payload packets"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			items := relayItems(1)
			items[0].Kind = tc.kind
			items[0].Packet.Payloads = make([]channeltypesv2.Payload, tc.payloads)
			txs, err := New(router, multicall).BuildRelayTxs(v2.ClientUpdate{}, items)
			require.ErrorContains(t, err, tc.want)
			require.Empty(t, txs)
		})
	}
}

func decodeMulticall(t *testing.T, data []byte) []ibcmulticall.IIBCMulticallMsgsCall {
	t.Helper()
	contractABI, err := ibcmulticall.ContractMetaData.GetAbi()
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(data), 4)
	require.Equal(t, contractABI.Methods["multicall"].ID, data[:4])
	args, err := contractABI.Methods["multicall"].Inputs.Unpack(data[4:])
	require.NoError(t, err)
	return *abi.ConvertType(args[0], new([]ibcmulticall.IIBCMulticallMsgsCall)).(*[]ibcmulticall.IIBCMulticallMsgsCall)
}

func relayItems(count int) []v2.PacketRelayItem {
	items := make([]v2.PacketRelayItem, count)
	for i := range items {
		items[i] = v2.PacketRelayItem{
			Kind: v2.RelayKindRecv,
			Packet: channeltypesv2.Packet{
				Sequence: uint64(i + 1), SourceClient: "source-0", DestinationClient: "dest-0",
				Payloads: []channeltypesv2.Payload{{SourcePort: "transfer", DestinationPort: "transfer"}},
			},
			Proof: []byte{1}, ProofHeight: 12,
		}
	}
	return items
}
