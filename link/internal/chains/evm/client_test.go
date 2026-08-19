// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/chains/evm/contracts/attestation"
	"github.com/cosmos/ibc/link/internal/tests/mocks"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

const (
	chainIDEth    = "1"
	routerAddress = "0xe20BccD900Fa1B48f46F5a483d9De063b07eDFCC"
)

var txHash = common.HexToHash("0x60016c34c02278856c81a41ce857ac4bb837a2f4a13c95207e08cbc9e8f2b706")

func testPacket() ics26router.IICS26RouterMsgsPacket {
	return ics26router.IICS26RouterMsgsPacket{
		Sequence:         42,
		SourceClient:     "base-0",
		DestClient:       "ethereum-0",
		TimeoutTimestamp: 1780000000,
		Payloads: []ics26router.IICS26RouterMsgsPayload{
			{
				SourcePort: "transfer",
				DestPort:   "transfer",
				Version:    "ics20-1",
				Encoding:   "application/x-solidity-abi",
				Value:      []byte{0xde, 0xad, 0xbe, 0xef},
			},
		},
	}
}

// sendPacketLog ABI-encodes a SendPacket event log as the router contract emits it.
func sendPacketLog(t *testing.T, address common.Address, packet ics26router.IICS26RouterMsgsPacket) *types.Log {
	t.Helper()

	routerABI, err := ics26router.ContractMetaData.GetAbi()
	require.NoError(t, err)

	event := routerABI.Events[sendPacketEvent]

	data, err := event.Inputs.NonIndexed().Pack(packet)
	require.NoError(t, err)

	return &types.Log{
		Address: address,
		Topics: []common.Hash{
			event.ID,
			crypto.Keccak256Hash([]byte(packet.SourceClient)),
			common.BigToHash(new(big.Int).SetUint64(packet.Sequence)),
		},
		Data: data,
	}
}

func TestTxPacketEvents(t *testing.T) {
	t.Run("parsesSendPacket", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		eth := mocks.NewMockETHClient(t)
		client, err := NewWithClient(chainIDEth, eth, routerAddress)
		require.NoError(t, err)

		packet := testPacket()
		receipt := &types.Receipt{
			BlockNumber: big.NewInt(100),
			Logs: []*types.Log{
				// unrelated contract emitting the same event is ignored
				sendPacketLog(t, common.HexToAddress("0x0000000000000000000000000000000000000bad"), packet),
				// unrelated log with no topics is ignored
				{Address: common.HexToAddress(routerAddress)},
				sendPacketLog(t, common.HexToAddress(routerAddress), packet),
			},
		}

		eth.EXPECT().TransactionReceipt(ctx, txHash).Return(receipt, nil).Once()
		eth.EXPECT().HeaderByNumber(ctx, big.NewInt(100)).Return(&types.Header{Time: 1752000000}, nil).Once()

		// ACT
		events, err := client.TxPacketEvents(ctx, txHash.Bytes())

		// ASSERT
		require.NoError(t, err)
		require.Len(t, events, 1)

		event := events[0]
		assert.Equal(t, v2.KindSendPacket, event.Kind)
		assert.Equal(t, uint64(100), event.Height)
		assert.Equal(t, time.Unix(1752000000, 0).UTC(), event.BlockTime)
		assert.Equal(t, uint64(42), event.Packet.Sequence)
		assert.Equal(t, "base-0", event.Packet.SourceClient)
		assert.Equal(t, "ethereum-0", event.Packet.DestinationClient)
		assert.Equal(t, uint64(1780000000), event.Packet.TimeoutTimestamp)
		require.Len(t, event.Packet.Payloads, 1)
		assert.Equal(t, "transfer", event.Packet.Payloads[0].SourcePort)
		assert.Equal(t, []byte{0xde, 0xad, 0xbe, 0xef}, event.Packet.Payloads[0].Value)
	})

	t.Run("parsesWriteAck", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		eth := mocks.NewMockETHClient(t)
		client, err := NewWithClient(chainIDEth, eth, routerAddress)
		require.NoError(t, err)

		packet := testPacket()
		ack := []byte{0xac, 0x01}
		receipt := &types.Receipt{
			BlockNumber: big.NewInt(100),
			Logs: []*types.Log{
				writeAckLog(t, common.HexToAddress(routerAddress), packet, [][]byte{ack}),
			},
		}

		eth.EXPECT().TransactionReceipt(ctx, txHash).Return(receipt, nil).Once()
		eth.EXPECT().HeaderByNumber(ctx, big.NewInt(100)).Return(&types.Header{Time: 1752000000}, nil).Once()

		// ACT
		events, err := client.TxPacketEvents(ctx, txHash.Bytes())

		// ASSERT
		require.NoError(t, err)
		require.Len(t, events, 1)

		event := events[0]
		assert.Equal(t, v2.KindWriteAck, event.Kind)
		assert.Equal(t, uint64(42), event.Packet.Sequence)
		require.Len(t, event.Acks, 1)
		assert.Equal(t, ack, event.Acks[0])
	})

	t.Run("noSendPackets", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		eth := mocks.NewMockETHClient(t)
		client, err := NewWithClient(chainIDEth, eth, routerAddress)
		require.NoError(t, err)

		receipt := &types.Receipt{BlockNumber: big.NewInt(100), Logs: []*types.Log{}}
		eth.EXPECT().TransactionReceipt(ctx, txHash).Return(receipt, nil).Once()
		// no HeaderByNumber expectation: it must not be called

		// ACT
		events, err := client.TxPacketEvents(ctx, txHash.Bytes())

		// ASSERT
		require.NoError(t, err)
		assert.Empty(t, events)
	})

	t.Run("headerError", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		eth := mocks.NewMockETHClient(t)
		client, err := NewWithClient(chainIDEth, eth, routerAddress)
		require.NoError(t, err)

		receipt := &types.Receipt{
			BlockNumber: big.NewInt(100),
			Logs:        []*types.Log{sendPacketLog(t, common.HexToAddress(routerAddress), testPacket())},
		}
		eth.EXPECT().TransactionReceipt(ctx, txHash).Return(receipt, nil).Once()
		eth.EXPECT().HeaderByNumber(ctx, big.NewInt(100)).Return(nil, errors.New("rpc down")).Once()

		// ACT
		_, err = client.TxPacketEvents(ctx, txHash.Bytes())

		// ASSERT
		require.ErrorContains(t, err, "getting header")
	})

	t.Run("receiptError", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		eth := mocks.NewMockETHClient(t)
		client, err := NewWithClient(chainIDEth, eth, routerAddress)
		require.NoError(t, err)

		eth.EXPECT().TransactionReceipt(ctx, txHash).Return(nil, assert.AnError).Once()

		// ACT
		_, err = client.TxPacketEvents(ctx, txHash.Bytes())

		// ASSERT
		require.ErrorContains(t, err, "getting receipt")
	})

	t.Run("invalidHashLength", func(t *testing.T) {
		// ARRANGE
		client, err := NewWithClient(chainIDEth, mocks.NewMockETHClient(t), routerAddress)
		require.NoError(t, err)

		// ACT
		_, err = client.TxPacketEvents(context.Background(), []byte{0xde, 0xad})

		// ASSERT
		require.ErrorContains(t, err, "invalid tx hash length")
	})

	t.Run("invalidRouterAddress", func(t *testing.T) {
		// ACT
		_, err := NewWithClient(chainIDEth, mocks.NewMockETHClient(t), "not-an-address")

		// ASSERT
		require.ErrorContains(t, err, "invalid ics26 router address")
	})
}

func TestGetBlockHeader(t *testing.T) {
	for _, tt := range []struct {
		name        string
		height      uint64
		rpcHeight   *big.Int
		header      *types.Header
		rpcErr      error
		expected    v2.BlockHeader
		errContains string
	}{
		{
			name:      "numericHeight",
			height:    42,
			rpcHeight: big.NewInt(42),
			header:    &types.Header{Number: big.NewInt(42), Time: 1_752_000_000},
			expected: v2.BlockHeader{
				Height:    42,
				Timestamp: time.Unix(1_752_000_000, 0).UTC(),
			},
		},
		{
			name:      "latestBlock",
			height:    v2.LatestBlock,
			rpcHeight: big.NewInt(rpc.LatestBlockNumber.Int64()),
			header:    &types.Header{Number: big.NewInt(101), Time: 1_752_000_001},
			expected: v2.BlockHeader{
				Height:    101,
				Timestamp: time.Unix(1_752_000_001, 0).UTC(),
			},
		},
		{
			name:      "finalizedBlock",
			height:    v2.FinalizedBlock,
			rpcHeight: big.NewInt(rpc.FinalizedBlockNumber.Int64()),
			header:    &types.Header{Number: big.NewInt(100), Time: 1_752_000_002},
			expected: v2.BlockHeader{
				Height:    100,
				Timestamp: time.Unix(1_752_000_002, 0).UTC(),
			},
		},
		{
			name:        "rpcError",
			height:      42,
			rpcHeight:   big.NewInt(42),
			rpcErr:      errors.New("rpc down"),
			errContains: "getting header for height 42: rpc down",
		},
		{
			name:        "nilHeader",
			height:      42,
			rpcHeight:   big.NewInt(42),
			errContains: "header is nil for height 42",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			ctx := context.Background()
			eth := mocks.NewMockETHClient(t)
			eth.EXPECT().HeaderByNumber(ctx, tt.rpcHeight).Return(tt.header, tt.rpcErr).Once()
			client, err := NewWithClient(chainIDEth, eth, routerAddress)
			require.NoError(t, err)

			// ACT
			actual, err := client.GetBlockHeader(ctx, tt.height)

			// ASSERT
			if tt.errContains != "" {
				require.ErrorContains(t, err, tt.errContains)
				require.Empty(t, actual)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expected, actual)
		})
	}
}

func TestGetCommitment(t *testing.T) {
	for _, tt := range []struct {
		name        string
		rpcErr      error
		errContains string
	}{
		{
			name: "returnsCommitmentAtHeight",
		},
		{
			name:        "rpcError",
			rpcErr:      errors.New("rpc down"),
			errContains: "getting commitment at height 42 on chain 1: rpc down",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			ctx := context.Background()
			eth := mocks.NewMockETHClient(t)
			client, err := NewWithClient(chainIDEth, eth, routerAddress)
			require.NoError(t, err)

			path := [32]byte{0xde, 0xad, 0xbe, 0xef}
			expected := [32]byte{0xca, 0xfe}
			routerABI, err := ics26router.ContractMetaData.GetAbi()
			require.NoError(t, err)
			callData, err := routerABI.Pack("getCommitment", path)
			require.NoError(t, err)
			output, err := routerABI.Methods["getCommitment"].Outputs.Pack(expected)
			require.NoError(t, err)
			if tt.rpcErr != nil {
				output = nil
			}
			address := common.HexToAddress(routerAddress)
			eth.EXPECT().CallContract(
				ctx,
				ethereum.CallMsg{To: &address, Data: callData},
				big.NewInt(42),
			).Return(output, tt.rpcErr).Once()

			// ACT
			actual, err := client.GetCommitment(ctx, 42, path)

			// ASSERT
			if tt.errContains != "" {
				require.ErrorContains(t, err, tt.errContains)
				assert.Empty(t, actual)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, expected, actual)
		})
	}
}

// writeAckLog ABI-encodes a WriteAcknowledgement event log as the router contract emits it.
func writeAckLog(
	t *testing.T,
	address common.Address,
	packet ics26router.IICS26RouterMsgsPacket,
	acks [][]byte,
) *types.Log {
	t.Helper()

	routerABI, err := ics26router.ContractMetaData.GetAbi()
	require.NoError(t, err)

	event := routerABI.Events[writeAckEvent]

	data, err := event.Inputs.NonIndexed().Pack(packet, acks)
	require.NoError(t, err)

	return &types.Log{
		Address: address,
		Topics: []common.Hash{
			event.ID,
			crypto.Keccak256Hash([]byte(packet.DestClient)),
			common.BigToHash(new(big.Int).SetUint64(packet.Sequence)),
		},
		Data: data,
	}
}

func newTestClient(t *testing.T) (*Client, *mocks.MockETHClient) {
	t.Helper()

	eth := mocks.NewMockETHClient(t)
	client, err := NewWithClient(chainIDEth, eth, routerAddress)
	require.NoError(t, err)

	return client, eth
}

func newSubscribingTestClient(t *testing.T) (*Client, *mocks.MockETHClient, *mocks.MockETHClient) {
	t.Helper()

	eth, ws := mocks.NewMockETHClient(t), mocks.NewMockETHClient(t)
	client, err := NewWithClients(chainIDEth, eth, ws, routerAddress)
	require.NoError(t, err)

	return client, eth, ws
}

// stubSubscription stands in for the live log subscription the ws client returns.
type stubSubscription struct{ errs chan error }

func (s stubSubscription) Err() <-chan error { return s.errs }
func (s stubSubscription) Unsubscribe()      {}

func TestPacketLogQuery(t *testing.T) {
	client, _ := newTestClient(t)

	routerABI, err := ics26router.ContractMetaData.GetAbi()
	require.NoError(t, err)

	t.Run("clientsAndSequences", func(t *testing.T) {
		query, err := client.packetLogQuery(
			sendPacketEvent,
			[]any{"base-0", "base-1"},
			[]any{uint64(7), uint64(9)},
		)

		require.NoError(t, err)
		assert.Equal(t, []common.Address{common.HexToAddress(routerAddress)}, query.Addresses)
		require.Len(t, query.Topics, 3)
		assert.Equal(t, []common.Hash{routerABI.Events[sendPacketEvent].ID}, query.Topics[0])
		assert.Equal(t, []common.Hash{
			crypto.Keccak256Hash([]byte("base-0")),
			crypto.Keccak256Hash([]byte("base-1")),
		}, query.Topics[1])
		assert.Equal(t, []common.Hash{
			common.BigToHash(big.NewInt(7)),
			common.BigToHash(big.NewInt(9)),
		}, query.Topics[2])
	})

	t.Run("emptySequencesAreAWildcard", func(t *testing.T) {
		query, err := client.packetLogQuery(sendPacketEvent, []any{"base-0"}, nil)

		require.NoError(t, err)
		require.Len(t, query.Topics, 3)
		assert.Nil(t, query.Topics[2])
	})
}

func TestSubscribeSendPackets(t *testing.T) {
	ctx := context.Background()

	// subscribe stands up the subscription against a stub ws subscription and
	// hands back the log sink the client is listening on.
	subscribe := func(
		t *testing.T,
		client *Client,
		ws *mocks.MockETHClient,
		out chan v2.PacketEvent,
	) (v2.Subscription, chan<- types.Log) {
		t.Helper()

		var sink chan<- types.Log

		ws.EXPECT().
			SubscribeFilterLogs(ctx, mock.Anything, mock.Anything).
			RunAndReturn(func(_ context.Context, _ ethereum.FilterQuery, ch chan<- types.Log) (ethereum.Subscription, error) {
				sink = ch
				return stubSubscription{errs: make(chan error, 1)}, nil
			}).
			Once()

		sub, err := client.SubscribeSendPackets(ctx, []string{"base-0"}, out)
		require.NoError(t, err)

		return sub, sink
	}

	t.Run("decodesSendPacket", func(t *testing.T) {
		client, eth, ws := newSubscribingTestClient(t)
		out := make(chan v2.PacketEvent, 2)
		sub, sink := subscribe(t, client, ws, out)

		defer sub.Unsubscribe()

		// once, not twice: both logs are in the same block
		eth.EXPECT().HeaderByNumber(ctx, big.NewInt(100)).Return(&types.Header{Time: 1752000000}, nil).Once()

		log := sendPacketLog(t, common.HexToAddress(routerAddress), testPacket())
		log.BlockNumber = 100
		log.TxHash = txHash

		sink <- *log
		sink <- *log

		for range 2 {
			packetEvent := <-out
			assert.Equal(t, v2.KindSendPacket, packetEvent.Kind)
			assert.Equal(t, txHash.String(), packetEvent.TxHash)
			assert.Equal(t, uint64(100), packetEvent.Height)
			assert.Equal(t, time.Unix(1752000000, 0).UTC(), packetEvent.BlockTime)
			assert.False(t, packetEvent.Removed)
			assert.Equal(t, uint64(42), packetEvent.Packet.Sequence)
			assert.Equal(t, "base-0", packetEvent.Packet.SourceClient)
		}
	})

	t.Run("headerErrorEndsSubscription", func(t *testing.T) {
		client, eth, ws := newSubscribingTestClient(t)
		out := make(chan v2.PacketEvent, 1)
		sub, sink := subscribe(t, client, ws, out)

		defer sub.Unsubscribe()

		eth.EXPECT().HeaderByNumber(ctx, big.NewInt(100)).Return(nil, errors.New("rpc down")).Once()

		log := sendPacketLog(t, common.HexToAddress(routerAddress), testPacket())
		log.BlockNumber = 100

		sink <- *log

		require.ErrorContains(t, <-sub.Err(), "rpc down")
		assert.Empty(t, out)
	})

	t.Run("subscribeError", func(t *testing.T) {
		client, _, ws := newSubscribingTestClient(t)
		ws.EXPECT().
			SubscribeFilterLogs(ctx, mock.Anything, mock.Anything).
			Return(nil, errors.New("ws down")).
			Once()

		_, err := client.SubscribeSendPackets(ctx, []string{"base-0"}, make(chan v2.PacketEvent))

		require.ErrorContains(t, err, "subscribing to SendPacket logs")
	})

	t.Run("withoutWebsocket", func(t *testing.T) {
		client, _ := newTestClient(t)

		_, err := client.SubscribeSendPackets(ctx, []string{"base-0"}, make(chan v2.PacketEvent))

		require.ErrorContains(t, err, "no websocket endpoint configured")
	})

	t.Run("withoutClientIDs", func(t *testing.T) {
		client, _, _ := newSubscribingTestClient(t)

		_, err := client.SubscribeSendPackets(ctx, nil, make(chan v2.PacketEvent))

		require.ErrorContains(t, err, "no client ids")
	})
}

func TestCommitmentQueries(t *testing.T) {
	ctx := context.Background()

	t.Run("packetNotReceived", func(t *testing.T) {
		client, eth := newTestClient(t)
		eth.EXPECT().CallContract(ctx, mock.Anything, (*big.Int)(nil)).Return(make([]byte, 32), nil).Once()

		received, err := client.IsPacketReceived(ctx, "base-0", 42)

		require.NoError(t, err)
		assert.False(t, received)
	})

	t.Run("packetCommitted", func(t *testing.T) {
		client, eth := newTestClient(t)
		commitment := make([]byte, 32)
		commitment[31] = 1
		eth.EXPECT().CallContract(ctx, mock.Anything, (*big.Int)(nil)).Return(commitment, nil).Once()

		committed, err := client.IsPacketCommitted(ctx, "base-0", 42)

		require.NoError(t, err)
		assert.True(t, committed)
	})
}

func TestFindPacketTx(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		client, eth := newTestClient(t)

		log := types.Log{TxHash: txHash, BlockNumber: 100}
		eth.EXPECT().FilterLogs(ctx, mock.Anything).Return([]types.Log{log}, nil).Once()
		eth.EXPECT().
			HeaderByNumber(ctx, big.NewInt(100)).
			Return(&types.Header{Time: 1752000000, Number: big.NewInt(100)}, nil).
			Once()
		// sender lookup failures are tolerated
		eth.EXPECT().TransactionByHash(ctx, txHash).Return(nil, false, errors.New("pruned")).Once()

		tx, err := client.FindAckTx(ctx, "base-0", 42)

		require.NoError(t, err)
		assert.Equal(t, txHash.String(), tx.Hash)
		assert.Equal(t, time.Unix(1752000000, 0).UTC(), tx.Timestamp)
		assert.Empty(t, tx.RelayerAddress)
	})

	t.Run("notFound", func(t *testing.T) {
		client, eth := newTestClient(t)
		eth.EXPECT().FilterLogs(ctx, mock.Anything).Return(nil, nil).Once()

		_, err := client.FindRecvTx(ctx, "base-0", 42)

		require.ErrorIs(t, err, v2.ErrTxNotFound)
	})

	t.Run("ambiguous", func(t *testing.T) {
		client, eth := newTestClient(t)
		eth.EXPECT().FilterLogs(ctx, mock.Anything).Return([]types.Log{{}, {}}, nil).Once()

		_, err := client.FindTimeoutTx(ctx, "base-0", 42)

		require.ErrorContains(t, err, "expected 1")
	})
}

func TestPacketWriteAckStatus(t *testing.T) {
	ctx := context.Background()
	packet := testPacket()

	receiptWithAcks := func(t *testing.T, acks [][]byte) *types.Receipt {
		t.Helper()

		return &types.Receipt{
			BlockNumber: big.NewInt(100),
			Logs:        []*types.Log{writeAckLog(t, common.HexToAddress(routerAddress), packet, acks)},
		}
	}

	t.Run("success", func(t *testing.T) {
		client, eth := newTestClient(t)
		eth.EXPECT().TransactionReceipt(ctx, txHash).Return(receiptWithAcks(t, [][]byte{{0x01}}), nil).Once()

		status, err := client.PacketWriteAckStatus(
			ctx,
			txHash.String(),
			packet.Sequence,
			packet.SourceClient,
			packet.DestClient,
		)

		require.NoError(t, err)
		assert.Equal(t, v2.WriteAckStatusSuccess, status)
	})

	t.Run("error", func(t *testing.T) {
		client, eth := newTestClient(t)
		eth.EXPECT().
			TransactionReceipt(ctx, txHash).
			Return(receiptWithAcks(t, [][]byte{errorAcknowledgement[:]}), nil).
			Once()

		status, err := client.PacketWriteAckStatus(
			ctx,
			txHash.String(),
			packet.Sequence,
			packet.SourceClient,
			packet.DestClient,
		)

		require.NoError(t, err)
		assert.Equal(t, v2.WriteAckStatusError, status)
	})

	t.Run("packetMismatch", func(t *testing.T) {
		client, eth := newTestClient(t)
		eth.EXPECT().TransactionReceipt(ctx, txHash).Return(receiptWithAcks(t, [][]byte{{0x01}}), nil).Once()

		_, err := client.PacketWriteAckStatus(
			ctx,
			txHash.String(),
			packet.Sequence+1,
			packet.SourceClient,
			packet.DestClient,
		)

		require.ErrorIs(t, err, v2.ErrWriteAckNotFoundForPacket)
	})

	t.Run("txNotFound", func(t *testing.T) {
		client, eth := newTestClient(t)
		eth.EXPECT().TransactionReceipt(ctx, txHash).Return(nil, ethereum.NotFound).Once()

		_, err := client.PacketWriteAckStatus(
			ctx,
			txHash.String(),
			packet.Sequence,
			packet.SourceClient,
			packet.DestClient,
		)

		require.ErrorIs(t, err, v2.ErrTxNotFound)
	})
}

func TestGetAttestationSet(t *testing.T) {
	lightClientAddress := common.HexToAddress("0x00000000000000000000000000000000000abc")

	routerABI, err := ics26router.ContractMetaData.GetAbi()
	require.NoError(t, err)

	attestationABI, err := attestation.ContractMetaData.GetAbi()
	require.NoError(t, err)

	getClientCallData, err := routerABI.Pack("getClient", "base-0")
	require.NoError(t, err)

	getClientOutput, err := routerABI.Methods["getClient"].Outputs.Pack(lightClientAddress)
	require.NoError(t, err)

	getAttestationSetCallData, err := attestationABI.Pack("getAttestationSet")
	require.NoError(t, err)

	expectedAddresses := []common.Address{
		common.HexToAddress("0x1111111111111111111111111111111111111111"),
		common.HexToAddress("0x2222222222222222222222222222222222222222"),
	}

	t.Run("returnsAddressesAndThreshold", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		client, eth := newTestClient(t)

		getAttestationSetOutput, err := attestationABI.Methods["getAttestationSet"].Outputs.Pack(
			expectedAddresses,
			uint8(2),
		)
		require.NoError(t, err)

		routerAddr := common.HexToAddress(routerAddress)
		eth.EXPECT().CallContract(
			ctx, ethereum.CallMsg{To: &routerAddr, Data: getClientCallData}, (*big.Int)(nil),
		).Return(getClientOutput, nil).Once()
		eth.EXPECT().CallContract(
			ctx, ethereum.CallMsg{To: &lightClientAddress, Data: getAttestationSetCallData}, (*big.Int)(nil),
		).Return(getAttestationSetOutput, nil).Once()

		// ACT
		addresses, minRequiredSigs, err := client.GetAttestationSet(ctx, "base-0")

		// ASSERT
		require.NoError(t, err)
		assert.Equal(t, []string{
			"0x1111111111111111111111111111111111111111",
			"0x2222222222222222222222222222222222222222",
		}, addresses)
		assert.Equal(t, uint8(2), minRequiredSigs)
	})

	t.Run("routerLookupError", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		client, eth := newTestClient(t)

		routerAddr := common.HexToAddress(routerAddress)
		eth.EXPECT().CallContract(
			ctx, ethereum.CallMsg{To: &routerAddr, Data: getClientCallData}, (*big.Int)(nil),
		).Return(nil, errors.New("rpc down")).Once()

		// ACT
		_, _, err := client.GetAttestationSet(ctx, "base-0")

		// ASSERT
		require.ErrorContains(t, err, "resolving light client address")
		require.ErrorContains(t, err, "rpc down")
	})

	t.Run("attestationSetQueryError", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		client, eth := newTestClient(t)

		routerAddr := common.HexToAddress(routerAddress)
		eth.EXPECT().CallContract(
			ctx, ethereum.CallMsg{To: &routerAddr, Data: getClientCallData}, (*big.Int)(nil),
		).Return(getClientOutput, nil).Once()
		eth.EXPECT().CallContract(
			ctx, ethereum.CallMsg{To: &lightClientAddress, Data: getAttestationSetCallData}, (*big.Int)(nil),
		).Return(nil, errors.New("rpc down")).Once()

		// ACT
		_, _, err := client.GetAttestationSet(ctx, "base-0")

		// ASSERT
		require.ErrorContains(t, err, "querying attestation set")
		require.ErrorContains(t, err, "rpc down")
	})
}

func TestWaitForChain(t *testing.T) {
	ctx := context.Background()

	client, eth := newTestClient(t)
	future := uint64(time.Now().Add(time.Hour).Unix())
	eth.EXPECT().
		HeaderByNumber(ctx, (*big.Int)(nil)).
		Return(&types.Header{Number: big.NewInt(1), Time: future}, nil).
		Once()

	require.NoError(t, client.WaitForChain(ctx))
}
