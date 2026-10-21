// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"math/big"
	"testing"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc-contracts/packages/go-abigen/ibcmulticall"
	"github.com/cosmos/ibc-contracts/packages/go-abigen/ics26router"
	"github.com/cosmos/ibc-contracts/packages/go-abigen/ift"
	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

const testSender = "0x0000000000000000000000000000000000001234"

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
	builder := New(router, multicall, successfulCaller(t))
	items := simulationPackets(3)
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
			txs, err := builder.BuildRelayTxs(t.Context(), testSender, update, items)
			require.NoError(t, err)
			require.Len(t, txs, 1)
			require.Equal(t, multicall.Bytes(), txs[0].To)
			calls := decodeMulticall(t, txs[0].Data)
			require.Len(t, calls, len(tc.updates)+len(items))
			methods := []string{"recvPacket", "ackPacket", "timeoutPacket"}
			for i, call := range calls {
				require.Equal(t, router, call.Target)
				require.True(t, call.AllowFailure)
				if i < len(tc.updates) {
					requireSelector(t, "updateClient", call.CallData)
					args, err := routerABI.Methods["updateClient"].Inputs.Unpack(call.CallData[4:])
					require.NoError(t, err)
					require.Equal(t, []any{update.ClientID, tc.updates[i]}, args)
				} else {
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
			items := simulationPackets(1)
			items[0].Kind = tc.kind
			items[0].Packet.Payloads = make([]channeltypesv2.Payload, tc.payloads)
			txs, err := New(router, multicall, nil).BuildRelayTxs(t.Context(), testSender, v2.ClientUpdate{}, items)
			require.ErrorContains(t, err, tc.want)
			require.Empty(t, txs)
		})
	}
	t.Run("missing multicall", func(t *testing.T) {
		txs, err := New(
			router,
			common.Address{},
			nil,
		).BuildRelayTxs(t.Context(), testSender, v2.ClientUpdate{}, simulationPackets(1))
		require.ErrorContains(t, err, "ibcMulticall is required")
		require.Empty(t, txs)
	})
	t.Run("invalid sender", func(t *testing.T) {
		txs, err := New(
			router,
			multicall,
			nil,
		).BuildRelayTxs(t.Context(), "bad address", v2.ClientUpdate{}, simulationPackets(1))
		require.ErrorContains(t, err, "invalid sender address")
		require.Empty(t, txs)
	})
}

func TestBuildRelayTxsSimulation(t *testing.T) {
	rpcErr := errors.New("rpc unavailable")
	for _, tc := range []struct {
		name             string
		updates, packets int
		success          []bool
		output           []byte
		err              error
		wantErr          string
	}{
		{name: "partial packet failure", updates: 2, packets: 2, success: []bool{true, true, false, true}},
		{name: "failed updates with packet success", updates: 2, packets: 2, success: []bool{false, false, true, false}},
		{name: "updates alone cannot qualify packets", updates: 2, packets: 2, success: []bool{true, true, false, false}, wantErr: "no packet calls"},
		{name: "all failed", updates: 1, packets: 2, success: []bool{false, false, false}, wantErr: "no packet calls"},
		{name: "no update needed", packets: 2, success: []bool{false, true}},
		{name: "update only", updates: 2, success: []bool{false, true}},
		{name: "failed update only", updates: 1, success: []bool{false}, wantErr: "no packet calls"},
		{name: "empty batch", wantErr: "no packet calls"},
		{name: "missing results", updates: 1, packets: 2, success: []bool{true}, wantErr: "result count"},
		{name: "RPC error", packets: 1, err: rpcErr, wantErr: "simulating multicall"},
		{name: "malformed result", packets: 1, output: []byte{1}, wantErr: "decoding multicall results"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			to := common.HexToAddress("0x2222")
			output := tc.output
			if output == nil {
				output = multicallOutput(t, tc.success...)
			}
			var simulated []byte
			simulations := 0
			caller := contractCallFunc(
				func(ctx context.Context, call ethereum.CallMsg, block *big.Int) ([]byte, error) {
					simulations++
					require.Equal(t, t.Context(), ctx)
					require.Equal(
						t,
						ethereum.CallMsg{From: common.HexToAddress(testSender), To: &to, Data: call.Data},
						call,
					)
					require.Equal(t, big.NewInt(-1), block)
					simulated = append([]byte(nil), call.Data...)
					return output, tc.err
				},
			)
			builder := New(common.HexToAddress("0x1111"), to, caller)
			update := v2.ClientUpdate{ClientID: "client-0", Payloads: make([][]byte, tc.updates)}
			txs, err := builder.BuildRelayTxs(t.Context(), testSender, update, simulationPackets(tc.packets))
			require.Equal(t, 1, simulations)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				if tc.err != nil {
					require.ErrorIs(t, err, tc.err)
				}
				require.Empty(t, txs)
				return
			}
			require.NoError(t, err)
			require.Equal(t, []v2.RelayTx{{To: to.Bytes(), Data: simulated}}, txs)
		})
	}
}

func TestBuildRelayTxsUsesSenderPerCall(t *testing.T) {
	senders := []string{testSender, "0x0000000000000000000000000000000000005678"}
	var got []common.Address
	caller := contractCallFunc(func(_ context.Context, call ethereum.CallMsg, _ *big.Int) ([]byte, error) {
		got = append(got, call.From)
		return multicallOutput(t, true), nil
	})
	builder := New(common.HexToAddress("0x1111"), common.HexToAddress("0x2222"), caller)
	for _, from := range senders {
		_, err := builder.BuildRelayTxs(t.Context(), from, v2.ClientUpdate{}, simulationPackets(1))
		require.NoError(t, err)
	}
	require.Equal(t, []common.Address{common.HexToAddress(senders[0]), common.HexToAddress(senders[1])}, got)
}

func TestBuildRelayTxsFailureDetails(t *testing.T) {
	contractABI, err := ibcmulticall.ContractMetaData.GetAbi()
	require.NoError(t, err)
	iftABI, err := ift.ContractMetaData.GetAbi()
	require.NoError(t, err)
	updateError := routerABI.Errors["IBCClientNotFound"]
	args, err := updateError.Inputs.Pack("client-0")
	require.NoError(t, err)
	limitError := iftABI.Errors["RateLimitExceeded"].ID
	for _, tc := range []struct {
		name    string
		success bool
	}{
		{"all packets fail", false},
		{"partial success", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// ARRANGE
			var logs bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
			t.Cleanup(func() { slog.SetDefault(previous) })
			output, err := contractABI.Methods["multicall"].Outputs.Pack([]ibcmulticall.IIBCMulticallMsgsResult{
				{ReturnData: append(updateError.ID[:4], args...)},
				{Success: true},
				{Success: tc.success},
				{ReturnData: limitError[:4]},
				{ReturnData: []byte{0xde, 0xad, 0xbe, 0xef}},
			})
			require.NoError(t, err)
			caller := contractCallFunc(func(context.Context, ethereum.CallMsg, *big.Int) ([]byte, error) {
				return output, nil
			})
			builder := New(common.HexToAddress("0x1111"), common.HexToAddress("0x2222"), caller)

			// ACT
			txs, err := builder.BuildRelayTxs(t.Context(), testSender,
				v2.ClientUpdate{ClientID: "client-0", Payloads: [][]byte{{1}, {2}}}, simulationPackets(3))

			// ASSERT
			var details string
			if tc.success {
				require.NoError(t, err)
				require.Len(t, txs, 1)
				details = logs.String()
			} else {
				require.ErrorContains(t, err, "no packet calls can currently succeed")
				require.Empty(t, txs)
				details = err.Error()
			}
			require.Contains(t, details, "call[0]: IBCClientNotFound(client-0)")
			require.Contains(t, details, "call[3]: RateLimitExceeded()")
			require.Contains(t, details, "call[4]: 0xdeadbeef")
			require.NotContains(t, details, "call[1]:")
		})
	}
}

func TestMulticallFailureDetailsBounded(t *testing.T) {
	contractABI, err := ibcmulticall.ContractMetaData.GetAbi()
	require.NoError(t, err)
	results := make([]ibcmulticall.IIBCMulticallMsgsResult, 8)
	for i := range results {
		results[i].ReturnData = bytes.Repeat([]byte{0xff}, 1024)
	}
	output, err := contractABI.Methods["multicall"].Outputs.Pack(results)
	require.NoError(t, err)
	caller := contractCallFunc(func(context.Context, ethereum.CallMsg, *big.Int) ([]byte, error) {
		return output, nil
	})
	builder := New(common.HexToAddress("0x1111"), common.HexToAddress("0x2222"), caller)
	err = builder.checkMulticall(t.Context(), common.HexToAddress(testSender), nil, 1, 7)
	require.Error(t, err)
	require.Contains(t, err.Error(), "...;")
	require.NotContains(t, err.Error(), "call[5]:")
	require.Less(t, len(err.Error()), 1500)
}

type contractCallFunc func(context.Context, ethereum.CallMsg, *big.Int) ([]byte, error)

func (f contractCallFunc) CallContract(ctx context.Context, call ethereum.CallMsg, block *big.Int) ([]byte, error) {
	return f(ctx, call, block)
}

func successfulCaller(t *testing.T) ContractCaller {
	t.Helper()
	return contractCallFunc(func(_ context.Context, call ethereum.CallMsg, _ *big.Int) ([]byte, error) {
		success := make([]bool, len(decodeMulticall(t, call.Data)))
		for i := range success {
			success[i] = true
		}
		return multicallOutput(t, success...), nil
	})
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

func multicallOutput(t *testing.T, success ...bool) []byte {
	t.Helper()
	contractABI, err := ibcmulticall.ContractMetaData.GetAbi()
	require.NoError(t, err)
	results := make([]ibcmulticall.IIBCMulticallMsgsResult, len(success))
	for i, ok := range success {
		results[i].Success = ok
	}
	output, err := contractABI.Methods["multicall"].Outputs.Pack(results)
	require.NoError(t, err)
	return output
}

func simulationPackets(count int) []v2.PacketRelayItem {
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
