package relayer

import (
	"context"
	"encoding/hex"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/store"
)

const (
	chainIDEth  = "1"
	chainIDBase = "8453"
	txHashLower = "0x60016c34c02278856c81a41ce857ac4bb837a2f4a13c95207e08cbc9e8f2b706"
	txHashUpper = "0x60016C34C02278856C81A41CE857AC4BB837A2F4A13C95207E08CBC9E8F2B706"
)

func relayerConfig() config.Config {
	return config.Config{
		Chains: []config.ChainConfig{
			{
				ChainID: chainIDEth,
				EVM: &config.EVMChainConfig{
					RPC: "https://ethereum-rpc.example.com",
					Contracts: config.EVMContracts{
						ICS26Router: "0x0000000000000000000000000000000000000000",
					},
				},
			},
		},
		Relayer: config.RelayerConfig{
			Clients: []config.ClientConfig{
				{
					ClientID:            "base-0",
					ChainID:             chainIDEth,
					CounterpartyChainID: chainIDBase,
					Type:                config.ClientTypeAttestation,
				},
			},
		},
	}
}

func txHashBytes(t *testing.T) []byte {
	t.Helper()

	raw, err := hex.DecodeString(txHashLower[2:])
	require.NoError(t, err)

	return raw
}

func TestRelay(t *testing.T) {
	t.Run("tracksExtractedPacketsAtomically", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		st := NewMockStore(t)
		repo := store.NewMockRepository(t)
		client := chains.NewMockClient(t)
		clientManager := NewMockChainClientManager(t)
		service := New(relayerConfig(), st, clientManager)

		blockTime := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
		events := []chains.PacketEvent{
			{
				Height:    100,
				BlockTime: blockTime,
				Kind:      chains.KindSendPacket,
				Packet: chains.Packet{
					Sequence:         42,
					SourceClient:     "base-0",
					DestClient:       "ethereum-0",
					TimeoutTimestamp: 1780000000,
				},
			},
			{
				// packets from unconfigured clients are skipped
				Height:    100,
				BlockTime: blockTime,
				Kind:      chains.KindSendPacket,
				Packet: chains.Packet{
					Sequence:     7,
					SourceClient: "unknown-0",
					DestClient:   "ethereum-0",
				},
			},
		}

		clientManager.EXPECT().GetClient(chainIDEth).Return(client, nil).Once()
		client.EXPECT().TxPacketEvents(ctx, txHashBytes(t)).Return(events, nil).Once()

		// request and transfers land in one transaction; hash normalized to lowercase
		st.EXPECT().
			ExecTx(ctx, mock.AnythingOfType("func(store.Repository) error")).
			RunAndReturn(func(ctx context.Context, fn func(store.Repository) error) error {
				return fn(repo)
			}).
			Once()
		repo.EXPECT().CreateRelayRequest(ctx, chainIDEth, txHashLower).Return(nil).Once()
		repo.EXPECT().CreateTransfer(ctx, store.Transfer{
			SourceChainID:             chainIDEth,
			DestinationChainID:        chainIDBase,
			SourceTxHash:              txHashLower,
			SourceTxTime:              blockTime,
			PacketSequenceNumber:      42,
			PacketSourceClientID:      "base-0",
			PacketDestinationClientID: "ethereum-0",
			PacketTimeoutTimestamp:    time.Unix(1780000000, 0).UTC(),
		}).Return(nil).Once()

		// ACT
		err := service.Relay(ctx, chainIDEth, txHashUpper)

		// ASSERT
		require.NoError(t, err)
	})

	t.Run("unsupportedChain", func(t *testing.T) {
		// ARRANGE
		service := New(relayerConfig(), NewMockStore(t), NewMockChainClientManager(t))

		// ACT
		err := service.Relay(context.Background(), "999", txHashLower)

		// ASSERT
		require.ErrorIs(t, err, ErrInvalidInput)
		require.ErrorContains(t, err, "unsupported chain")
	})

	t.Run("extractionError", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		client := chains.NewMockClient(t)
		clientManager := NewMockChainClientManager(t)
		service := New(relayerConfig(), NewMockStore(t), clientManager)

		// nothing is recorded when extraction fails
		clientManager.EXPECT().GetClient(chainIDEth).Return(client, nil).Once()
		client.EXPECT().TxPacketEvents(ctx, txHashBytes(t)).Return(nil, errors.New("rpc down")).Once()

		// ACT
		err := service.Relay(ctx, chainIDEth, txHashLower)

		// ASSERT
		require.ErrorContains(t, err, "extracting packet events")
		require.NotErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("validation", func(t *testing.T) {
		for _, tt := range []struct {
			name    string
			chainID string
			txHash  string
		}{
			{name: "empty chainID", chainID: "", txHash: txHashLower},
			{name: "empty txHash", chainID: chainIDEth, txHash: ""},
			{name: "not hex", chainID: chainIDEth, txHash: "0xnothex"},
			{name: "too short", chainID: chainIDEth, txHash: "0xdeadbeef"},
			{name: "missing prefix", chainID: chainIDEth, txHash: txHashLower[2:] + "00"},
		} {
			t.Run(tt.name, func(t *testing.T) {
				// ARRANGE
				service := New(relayerConfig(), NewMockStore(t), NewMockChainClientManager(t))

				// ACT
				err := service.Relay(context.Background(), tt.chainID, tt.txHash)

				// ASSERT
				require.ErrorIs(t, err, ErrInvalidInput)
			})
		}
	})

	t.Run("storeError", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		st := NewMockStore(t)
		client := chains.NewMockClient(t)
		clientManager := NewMockChainClientManager(t)
		service := New(relayerConfig(), st, clientManager)

		clientManager.EXPECT().GetClient(chainIDEth).Return(client, nil).Once()
		client.EXPECT().TxPacketEvents(ctx, txHashBytes(t)).Return(nil, nil).Once()
		st.EXPECT().
			ExecTx(ctx, mock.AnythingOfType("func(store.Repository) error")).
			Return(errors.New("boom")).
			Once()

		// ACT
		err := service.Relay(ctx, chainIDEth, txHashLower)

		// ASSERT
		require.ErrorContains(t, err, "recording relay request")
		require.NotErrorIs(t, err, ErrInvalidInput)
	})
}

func TestStatus(t *testing.T) {
	t.Run("notSubmitted", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		st := NewMockStore(t)
		service := New(relayerConfig(), st, NewMockChainClientManager(t))

		st.EXPECT().GetRelayRequest(ctx, chainIDEth, txHashLower).Return(nil, store.ErrNotFound).Once()

		// ACT
		_, err := service.Status(ctx, chainIDEth, txHashLower)

		// ASSERT
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("submittedWithoutTransfers", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		st := NewMockStore(t)
		service := New(relayerConfig(), st, NewMockChainClientManager(t))

		st.EXPECT().GetRelayRequest(ctx, chainIDEth, txHashLower).Return(&store.RelayRequest{ID: 1}, nil).Once()
		st.EXPECT().ListTransfersBySourceTx(ctx, chainIDEth, txHashLower).Return(nil, nil).Once()

		// ACT
		statuses, err := service.Status(ctx, chainIDEth, txHashLower)

		// ASSERT
		require.NoError(t, err)
		assert.Empty(t, statuses)
	})

	t.Run("mapsTransfers", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		st := NewMockStore(t)
		service := New(relayerConfig(), st, NewMockChainClientManager(t))

		recvTxHash := "0xrecv"
		transfers := []store.Transfer{
			{
				Status:               store.TransferStatusDeliverRecvPacket,
				PacketSequenceNumber: 42,
				PacketSourceClientID: "base-0",
				SourceChainID:        chainIDEth,
				DestinationChainID:   chainIDBase,
				SourceTxHash:         txHashLower,
				RecvTxHash:           &recvTxHash,
			},
			{
				Status:               store.TransferStatusCompleteWithAck,
				PacketSequenceNumber: 43,
				PacketSourceClientID: "base-0",
				SourceChainID:        chainIDEth,
				DestinationChainID:   chainIDBase,
				SourceTxHash:         txHashLower,
			},
		}

		st.EXPECT().GetRelayRequest(ctx, chainIDEth, txHashLower).Return(&store.RelayRequest{ID: 1}, nil).Once()
		st.EXPECT().ListTransfersBySourceTx(ctx, chainIDEth, txHashLower).Return(transfers, nil).Once()

		// ACT
		statuses, err := service.Status(ctx, chainIDEth, txHashUpper)

		// ASSERT
		require.NoError(t, err)
		require.Len(t, statuses, 2)

		first := statuses[0]
		assert.Equal(t, StatePending, first.State)
		assert.Equal(t, uint64(42), first.SequenceNumber)
		assert.Equal(t, "base-0", first.SourceClientID)
		assert.Equal(t, TxInfo{TxHash: txHashLower, ChainID: chainIDEth}, first.SendTx)
		require.NotNil(t, first.RecvTx)
		assert.Equal(t, TxInfo{TxHash: recvTxHash, ChainID: chainIDBase}, *first.RecvTx)
		assert.Nil(t, first.AckTx)
		assert.Nil(t, first.TimeoutTx)

		assert.Equal(t, StateComplete, statuses[1].State)
		assert.Nil(t, statuses[1].RecvTx)
	})
}

func TestMapTransferState(t *testing.T) {
	pending := []store.TransferStatus{
		store.TransferStatusPending,
		store.TransferStatusAwaitingSendFinality,
		store.TransferStatusCheckRecvPacketDelivery,
		store.TransferStatusGetRecvPacket,
		store.TransferStatusDeliverRecvPacket,
		store.TransferStatusWaitForWriteAck,
		store.TransferStatusAwaitingWriteAckFinality,
		store.TransferStatusCheckAckPacketDelivery,
		store.TransferStatusGetAckPacket,
		store.TransferStatusDeliverAckPacket,
		store.TransferStatusAwaitingTimeoutFinality,
		store.TransferStatusCheckTimeoutPacketDelivery,
		store.TransferStatusGetTimeoutPacket,
		store.TransferStatusDeliverTimeoutPacket,
	}
	for _, status := range pending {
		assert.Equal(t, StatePending, mapTransferState(status), string(status))
	}

	complete := []store.TransferStatus{
		store.TransferStatusCompleteWithAck,
		store.TransferStatusCompleteWithWriteAckSuccess,
		store.TransferStatusCompleteWithWriteAckError,
		store.TransferStatusCompleteWithTimeout,
	}
	for _, status := range complete {
		assert.Equal(t, StateComplete, mapTransferState(status), string(status))
	}

	assert.Equal(t, StateFailed, mapTransferState(store.TransferStatusFailed))
}
