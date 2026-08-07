package relayer

import (
	"context"
	"encoding/hex"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"

	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/store"
	"github.com/cosmos/ibc/link/internal/tests/mocks"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
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
					RPC:         "https://ethereum-rpc.example.com",
					ICS26Router: "0x0000000000000000000000000000000000000000",
				},
			},
		},
		Relayer: config.RelayerConfig{
			Connections: []config.ConnectionConfig{
				{
					ClientA: config.ClientEndConfig{
						ClientID: "base-0",
						ChainID:  chainIDEth,
						Type:     config.ClientTypeAttestation,
					},
					ClientB: config.ClientEndConfig{
						ChainID: chainIDBase,
						Type:    config.ClientTypeAttestation,
					},
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
		repo := mocks.NewMockRepository(t)
		client := mocks.NewMockClient(t)
		clients := NewMockChainClients(t)
		service := New(relayerConfig(), st, clients, nil)

		blockTime := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
		events := []v2.PacketEvent{
			{
				Height:    100,
				BlockTime: blockTime,
				Kind:      v2.KindSendPacket,
				Packet: channeltypesv2.Packet{
					Sequence:          42,
					SourceClient:      "base-0",
					DestinationClient: "ethereum-0",
					TimeoutTimestamp:  1780000000,
				},
			},
			{
				// packets from unconfigured clients are skipped
				Height:    100,
				BlockTime: blockTime,
				Kind:      v2.KindSendPacket,
				Packet: channeltypesv2.Packet{
					Sequence:          7,
					SourceClient:      "unknown-0",
					DestinationClient: "ethereum-0",
				},
			},
		}

		clients.EXPECT().Get(chainIDEth).Return(client, true).Once()
		client.EXPECT().TxPacketEvents(ctx, txHashBytes(t)).Return(events, nil).Once()

		// request and packets land in one transaction; hash normalized to lowercase
		st.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(store.Repository) error")).
			RunAndReturn(func(ctx context.Context, fn func(store.Repository) error) error {
				return fn(repo)
			}).
			Once()
		repo.EXPECT().CreateRelayRequest(ctx, chainIDEth, txHashLower).Return(nil).Once()
		repo.EXPECT().CreatePacket(ctx, store.CreatePacket{
			Status:                    store.RelayStatusPending,
			SourceChainID:             chainIDEth,
			DestinationChainID:        chainIDBase,
			SourceTxHash:              txHashLower,
			SourceTxTime:              blockTime,
			PacketSequenceNumber:      42,
			PacketSourceClientID:      "base-0",
			PacketDestinationClientID: "ethereum-0",
			PacketTimeoutTimestamp:    time.Unix(1780000000, 0).UTC(),
		}).Return(nil).Once()

		// we do not expect CreatePacket to be called for "unknown-0"

		// ACT
		err := service.Relay(ctx, chainIDEth, txHashUpper)

		// ASSERT
		require.NoError(t, err)
	})

	t.Run("unsupportedChain", func(t *testing.T) {
		// ARRANGE
		service := New(relayerConfig(), NewMockStore(t), NewMockChainClients(t), nil)

		// ACT
		err := service.Relay(context.Background(), "999", txHashLower)

		// ASSERT
		require.ErrorIs(t, err, ErrInvalidInput)
		require.ErrorContains(t, err, "unsupported chain")
	})

	t.Run("chainClientError", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		clients := NewMockChainClients(t)
		service := New(relayerConfig(), NewMockStore(t), clients, nil)

		// config knows the chain but the chain client set has no client for it
		clients.EXPECT().Get(chainIDEth).Return(nil, false).Once()

		// ACT
		err := service.Relay(ctx, chainIDEth, txHashLower)

		// ASSERT
		// a missing client is a server-side inconsistency, not a caller error
		require.ErrorContains(t, err, "client for chain")
		require.ErrorIs(t, err, ErrNotFound)
		require.NotErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("unknownTransaction", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		client := mocks.NewMockClient(t)
		clients := NewMockChainClients(t)
		service := New(relayerConfig(), NewMockStore(t), clients, nil)

		clients.EXPECT().Get(chainIDEth).Return(client, true).Once()
		client.EXPECT().TxPacketEvents(ctx, txHashBytes(t)).Return(nil, ethereum.NotFound).Once()

		// ACT
		err := service.Relay(ctx, chainIDEth, txHashLower)

		// ASSERT
		require.ErrorIs(t, err, ErrNotFound)
		require.ErrorContains(t, err, "no packets found")
	})

	t.Run("extractionError", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		client := mocks.NewMockClient(t)
		clients := NewMockChainClients(t)
		service := New(relayerConfig(), NewMockStore(t), clients, nil)

		// nothing is recorded when extraction fails
		clients.EXPECT().Get(chainIDEth).Return(client, true).Once()
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
				service := New(relayerConfig(), NewMockStore(t), NewMockChainClients(t), nil)

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
		client := mocks.NewMockClient(t)
		clients := NewMockChainClients(t)
		service := New(relayerConfig(), st, clients, nil)

		clients.EXPECT().Get(chainIDEth).Return(client, true).Once()
		client.EXPECT().TxPacketEvents(ctx, txHashBytes(t)).Return(nil, nil).Once()
		st.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(store.Repository) error")).
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
		service := New(relayerConfig(), st, NewMockChainClients(t), nil)

		st.EXPECT().GetRelayRequest(ctx, chainIDEth, txHashLower).Return(nil, store.ErrNotFound).Once()

		// ACT
		_, err := service.Status(ctx, chainIDEth, txHashLower)

		// ASSERT
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("submittedWithoutPackets", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		st := NewMockStore(t)
		service := New(relayerConfig(), st, NewMockChainClients(t), nil)

		st.EXPECT().GetRelayRequest(ctx, chainIDEth, txHashLower).Return(&store.RelayRequest{ID: 1}, nil).Once()
		st.EXPECT().ListPacketsBySourceTx(ctx, chainIDEth, txHashLower).Return(nil, nil).Once()

		// ACT
		statuses, err := service.Status(ctx, chainIDEth, txHashLower)

		// ASSERT
		require.NoError(t, err)
		assert.Empty(t, statuses)
	})

	t.Run("mapsPackets", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		st := NewMockStore(t)
		service := New(relayerConfig(), st, NewMockChainClients(t), nil)

		recvTxHash := "0xrecv"
		ackTxHash := "0xack"
		packets := []store.Packet{
			{
				Status:               store.RelayStatusDeliverRecvPacket,
				PacketSequenceNumber: 42,
				PacketSourceClientID: "base-0",
				SourceChainID:        chainIDEth,
				DestinationChainID:   chainIDBase,
				SourceTxHash:         txHashLower,
				RecvTxHash:           &recvTxHash,
			},
			{
				Status:               store.RelayStatusCompleteWithAck,
				PacketSequenceNumber: 43,
				PacketSourceClientID: "base-0",
				SourceChainID:        chainIDEth,
				DestinationChainID:   chainIDBase,
				SourceTxHash:         txHashLower,
				RecvTxHash:           &recvTxHash,
				AckTxHash:            &ackTxHash,
			},
		}

		st.EXPECT().GetRelayRequest(ctx, chainIDEth, txHashLower).Return(&store.RelayRequest{ID: 1}, nil).Once()
		st.EXPECT().ListPacketsBySourceTx(ctx, chainIDEth, txHashLower).Return(packets, nil).Once()

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

		assert.Equal(t, StateSucceeded, statuses[1].State)
		require.NotNil(t, statuses[1].RecvTx)
		require.NotNil(t, statuses[1].AckTx)
	})
}

func TestMapPacketState(t *testing.T) {
	pending := []store.RelayStatus{
		store.RelayStatusPending,
		store.RelayStatusAwaitingSendFinality,
		store.RelayStatusCheckRecvPacketDelivery,
		store.RelayStatusGetRecvPacket,
		store.RelayStatusDeliverRecvPacket,
		store.RelayStatusWaitForWriteAck,
		store.RelayStatusAwaitingWriteAckFinality,
		store.RelayStatusCheckAckPacketDelivery,
		store.RelayStatusGetAckPacket,
		store.RelayStatusDeliverAckPacket,
		store.RelayStatusAwaitingTimeoutFinality,
		store.RelayStatusCheckTimeoutPacketDelivery,
		store.RelayStatusGetTimeoutPacket,
		store.RelayStatusDeliverTimeoutPacket,
	}
	for _, status := range pending {
		assert.Equal(t, StatePending, mapPacketState(status), string(status))
	}

	assert.Equal(t, StateSucceeded, mapPacketState(store.RelayStatusCompleteWithAck))
	assert.Equal(t, StateTimedOut, mapPacketState(store.RelayStatusCompleteWithTimeout))
	assert.Equal(t, StateRejected, mapPacketState(store.RelayStatusCompleteWithWriteAckError))
	assert.Equal(t, StateRelayFailed, mapPacketState(store.RelayStatusFailed))
}
