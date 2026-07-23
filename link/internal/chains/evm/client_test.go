package evm

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/chains/evm/contracts/ics26router"
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
		assert.Equal(t, "ethereum-0", event.Packet.DestClient)
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
func writeAckLog(t *testing.T, address common.Address, packet ics26router.IICS26RouterMsgsPacket, acks [][]byte) *types.Log {
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
		eth.EXPECT().HeaderByNumber(ctx, big.NewInt(100)).Return(&types.Header{Time: 1752000000, Number: big.NewInt(100)}, nil).Once()
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

		status, err := client.PacketWriteAckStatus(ctx, txHash.String(), packet.Sequence, packet.SourceClient, packet.DestClient)

		require.NoError(t, err)
		assert.Equal(t, v2.WriteAckStatusSuccess, status)
	})

	t.Run("error", func(t *testing.T) {
		client, eth := newTestClient(t)
		eth.EXPECT().TransactionReceipt(ctx, txHash).Return(receiptWithAcks(t, [][]byte{errorAcknowledgement[:]}), nil).Once()

		status, err := client.PacketWriteAckStatus(ctx, txHash.String(), packet.Sequence, packet.SourceClient, packet.DestClient)

		require.NoError(t, err)
		assert.Equal(t, v2.WriteAckStatusError, status)
	})

	t.Run("packetMismatch", func(t *testing.T) {
		client, eth := newTestClient(t)
		eth.EXPECT().TransactionReceipt(ctx, txHash).Return(receiptWithAcks(t, [][]byte{{0x01}}), nil).Once()

		_, err := client.PacketWriteAckStatus(ctx, txHash.String(), packet.Sequence+1, packet.SourceClient, packet.DestClient)

		require.ErrorIs(t, err, v2.ErrWriteAckNotFoundForPacket)
	})

	t.Run("txNotFound", func(t *testing.T) {
		client, eth := newTestClient(t)
		eth.EXPECT().TransactionReceipt(ctx, txHash).Return(nil, ethereum.NotFound).Once()

		_, err := client.PacketWriteAckStatus(ctx, txHash.String(), packet.Sequence, packet.SourceClient, packet.DestClient)

		require.ErrorIs(t, err, v2.ErrTxNotFound)
	})
}

func TestFinality(t *testing.T) {
	ctx := context.Background()
	offset := uint64(5)

	t.Run("txFinalizedWithOffset", func(t *testing.T) {
		client, eth := newTestClient(t)
		eth.EXPECT().TransactionReceipt(ctx, txHash).Return(&types.Receipt{BlockNumber: big.NewInt(100)}, nil).Twice()
		eth.EXPECT().HeaderByNumber(ctx, (*big.Int)(nil)).Return(&types.Header{Number: big.NewInt(105), Time: 1}, nil).Once()

		finalized, err := client.IsTxFinalized(ctx, txHash.String(), &offset)
		require.NoError(t, err)
		assert.True(t, finalized)

		eth.EXPECT().HeaderByNumber(ctx, (*big.Int)(nil)).Return(&types.Header{Number: big.NewInt(104), Time: 1}, nil).Once()

		finalized, err = client.IsTxFinalized(ctx, txHash.String(), &offset)
		require.NoError(t, err)
		assert.False(t, finalized)
	})

	t.Run("txFinalizedNative", func(t *testing.T) {
		client, eth := newTestClient(t)
		eth.EXPECT().TransactionReceipt(ctx, txHash).Return(&types.Receipt{BlockNumber: big.NewInt(100)}, nil).Once()
		eth.EXPECT().HeaderByNumber(ctx, big.NewInt(rpc.FinalizedBlockNumber.Int64())).Return(&types.Header{Number: big.NewInt(100), Time: 1}, nil).Once()

		finalized, err := client.IsTxFinalized(ctx, txHash.String(), nil)

		require.NoError(t, err)
		assert.True(t, finalized)
	})

	t.Run("timestampFinalizedWithOffset", func(t *testing.T) {
		client, eth := newTestClient(t)
		eth.EXPECT().HeaderByNumber(ctx, (*big.Int)(nil)).Return(&types.Header{Number: big.NewInt(100), Time: 1}, nil).Once()
		eth.EXPECT().HeaderByNumber(ctx, big.NewInt(95)).Return(&types.Header{Number: big.NewInt(95), Time: 1752000000}, nil).Once()

		finalized, err := client.IsTimestampFinalized(ctx, time.Unix(1751999999, 0), &offset)

		require.NoError(t, err)
		assert.True(t, finalized)
	})
}

func TestWaitForChain(t *testing.T) {
	ctx := context.Background()

	client, eth := newTestClient(t)
	future := uint64(time.Now().Add(time.Hour).Unix())
	eth.EXPECT().HeaderByNumber(ctx, (*big.Int)(nil)).Return(&types.Header{Number: big.NewInt(1), Time: future}, nil).Once()

	require.NoError(t, client.WaitForChain(ctx))
}
