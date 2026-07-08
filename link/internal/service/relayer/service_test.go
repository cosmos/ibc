package relayer

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/store"
)

const (
	chainIDEth  = "1"
	chainIDBase = "8453"
	txHashLower = "0x60016c34c02278856c81a41ce857ac4bb837a2f4a13c95207e08cbc9e8f2b706"
	txHashUpper = "0x60016C34C02278856C81A41CE857AC4BB837A2F4A13C95207E08CBC9E8F2B706"
)

func TestRelay(t *testing.T) {
	t.Run("recordsRelayRequest", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		st := NewMockStore(t)
		service := New(config.RelayerConfig{}, st)

		// tx hash is normalized to lowercase before storage
		st.EXPECT().UpsertRelayRequest(ctx, chainIDEth, txHashLower).Return(nil).Once()

		// ACT
		err := service.Relay(ctx, chainIDEth, txHashUpper)

		// ASSERT
		require.NoError(t, err)
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
				service := New(config.RelayerConfig{}, NewMockStore(t))

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
		service := New(config.RelayerConfig{}, st)

		st.EXPECT().UpsertRelayRequest(ctx, chainIDEth, txHashLower).Return(errors.New("boom")).Once()

		// ACT
		err := service.Relay(ctx, chainIDEth, txHashLower)

		// ASSERT
		require.ErrorContains(t, err, "upserting relay request")
		require.NotErrorIs(t, err, ErrInvalidInput)
	})
}

func TestStatus(t *testing.T) {
	t.Run("notSubmitted", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		st := NewMockStore(t)
		service := New(config.RelayerConfig{}, st)

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
		service := New(config.RelayerConfig{}, st)

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
		service := New(config.RelayerConfig{}, st)

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
