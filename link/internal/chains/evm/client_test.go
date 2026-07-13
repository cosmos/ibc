package evm

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/chains"
	ics26router "github.com/cosmos/ibc/link/internal/chains/evm/contracts/ics26_router"
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
		eth := NewMockETHClient(t)
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
		assert.Equal(t, chains.KindSendPacket, event.Kind)
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

	t.Run("noSendPacketsSkipsHeaderLookup", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		eth := NewMockETHClient(t)
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

	t.Run("receiptError", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		eth := NewMockETHClient(t)
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
		client, err := NewWithClient(chainIDEth, NewMockETHClient(t), routerAddress)
		require.NoError(t, err)

		// ACT
		_, err = client.TxPacketEvents(context.Background(), []byte{0xde, 0xad})

		// ASSERT
		require.ErrorContains(t, err, "invalid tx hash length")
	})

	t.Run("invalidRouterAddress", func(t *testing.T) {
		// ACT
		_, err := NewWithClient(chainIDEth, NewMockETHClient(t), "not-an-address")

		// ASSERT
		require.ErrorContains(t, err, "invalid ics26 router address")
	})

}
