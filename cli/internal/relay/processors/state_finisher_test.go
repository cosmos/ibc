// SPDX-License-Identifier: Apache-2.0

package processors

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/cosmos/ibc/cli/internal/store"
)

type fakeStatusStorage struct {
	last store.RelayStatus
	err  error
}

func (f *fakeStatusStorage) UpdatePacketStatus(_ context.Context, _ store.PacketKey, status store.RelayStatus) error {
	if f.err != nil {
		return f.err
	}

	f.last = status

	return nil
}

func TestStateFinisherProcess(t *testing.T) {
	base := func() *Transfer {
		return NewTransfer(store.Packet{
			PacketTimeoutTimestamp: time.Now().Add(time.Hour),
		}, slog.Default())
	}

	t.Run("timeout", func(t *testing.T) {
		tr := base()
		hash := "0xtimeout"
		tr.TimeoutTxHash = &hash

		storage := &fakeStatusStorage{}
		_, err := NewStateFinisher(storage).Process(context.Background(), tr)
		require.NoError(t, err)

		assert.Equal(t, store.RelayStatusCompleteWithTimeout, tr.Status)
		assert.Equal(t, store.RelayStatusCompleteWithTimeout, storage.last)
	})

	t.Run("ackRelayedWithWriteAckSuccess", func(t *testing.T) {
		tr := base()
		recvHash, writeAckHash, ackHash := "0xrecv", "0xwriteack", "0xack"
		status := store.WriteAckStatusSuccess
		tr.RecvTxHash = &recvHash
		tr.WriteAckTxHash = &writeAckHash
		tr.WriteAckStatus = &status
		tr.AckTxHash = &ackHash

		storage := &fakeStatusStorage{}
		_, err := NewStateFinisher(storage).Process(context.Background(), tr)
		require.NoError(t, err)

		assert.Equal(t, store.RelayStatusCompleteWithAck, tr.Status)
	})

	t.Run("ackRelayedWithWriteAckError", func(t *testing.T) {
		tr := base()
		recvHash, writeAckHash, ackHash := "0xrecv", "0xwriteack", "0xack"
		status := store.WriteAckStatusError
		tr.RecvTxHash = &recvHash
		tr.WriteAckTxHash = &writeAckHash
		tr.WriteAckStatus = &status
		tr.AckTxHash = &ackHash

		storage := &fakeStatusStorage{}
		_, err := NewStateFinisher(storage).Process(context.Background(), tr)
		require.NoError(t, err)

		assert.Equal(t, store.RelayStatusCompleteWithWriteAckError, tr.Status)
		assert.Equal(t, store.RelayStatusCompleteWithWriteAckError, storage.last)
	})

	t.Run("ackRelayedWithUnknownWriteAckStatus", func(t *testing.T) {
		tr := base()
		recvHash, writeAckHash, ackHash := "0xrecv", "0xwriteack", "0xack"
		status := store.WriteAckStatusUnknown
		tr.RecvTxHash = &recvHash
		tr.WriteAckTxHash = &writeAckHash
		tr.WriteAckStatus = &status
		tr.AckTxHash = &ackHash

		storage := &fakeStatusStorage{}
		_, err := NewStateFinisher(storage).Process(context.Background(), tr)
		require.NoError(t, err)

		assert.Equal(t, store.RelayStatusCompleteWithAck, tr.Status)
	})

	t.Run("ackRelayedWithNoRecordedWriteAckStatus", func(t *testing.T) {
		tr := base()
		recvHash, writeAckHash, ackHash := "0xrecv", "0xwriteack", "0xack"
		tr.RecvTxHash = &recvHash
		tr.WriteAckTxHash = &writeAckHash
		tr.AckTxHash = &ackHash

		storage := &fakeStatusStorage{}
		_, err := NewStateFinisher(storage).Process(context.Background(), tr)
		require.NoError(t, err)

		assert.Equal(t, store.RelayStatusCompleteWithAck, tr.Status)
	})

	t.Run("notComplete", func(t *testing.T) {
		tr := base()

		storage := &fakeStatusStorage{}
		_, err := NewStateFinisher(storage).Process(context.Background(), tr)
		require.NoError(t, err)

		assert.Empty(t, tr.Status)
		assert.Empty(t, storage.last)
	})
}

func TestStateFinisherCompletionMetrics(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() {
		require.NoError(t, provider.Shutdown(context.Background()))
	})

	instruments, err := newInstrumentation(provider.Meter("test"))
	require.NoError(t, err)

	originalMetrics := metrics
	metrics = instruments
	t.Cleanup(func() {
		metrics = originalMetrics
	})

	ctx := context.Background()
	finisher := NewStateFinisher(&fakeStatusStorage{})
	metricCount := func(name string) uint64 {
		var data metricdata.ResourceMetrics
		require.NoError(t, reader.Collect(ctx, &data))

		var count uint64
		for _, scope := range data.ScopeMetrics {
			for _, metric := range scope.Metrics {
				if metric.Name != name {
					continue
				}

				sum, ok := metric.Data.(metricdata.Sum[int64])
				require.True(t, ok)
				for _, point := range sum.DataPoints {
					count += uint64(point.Value)
				}
			}
		}

		return count
	}

	t.Run("retryAndNonTerminalTransfersAreNotCounted", func(t *testing.T) {
		tr := NewTransfer(store.Packet{
			Status:                 store.RelayStatusDeliverRecvPacket,
			PacketTimeoutTimestamp: time.Now().Add(time.Hour),
		}, slog.Default())
		tr.ProcessingError = ErrRetryingRecvPacket

		_, err := finisher.Process(ctx, tr)
		require.NoError(t, err)
		assert.Zero(t, metricCount("relays_completed_total"))
	})

	t.Run("failedTerminalPersistenceIsNotCounted", func(t *testing.T) {
		timeoutHash := "0xtimeout-error"
		tr := NewTransfer(store.Packet{
			Status:                 store.RelayStatusDeliverTimeoutPacket,
			PacketTimeoutTimestamp: time.Now().Add(-time.Hour),
			TimeoutTxHash:          &timeoutHash,
		}, slog.Default())

		_, err := NewStateFinisher(&fakeStatusStorage{err: assert.AnError}).Process(ctx, tr)
		require.NoError(t, err)
		require.ErrorIs(t, tr.ProcessingError, assert.AnError)
		assert.Equal(t, store.RelayStatusDeliverTimeoutPacket, tr.Status)
		assert.Zero(t, metricCount("relays_completed_total"))
	})

	t.Run("successfulTerminalTransferIsCountedOnce", func(t *testing.T) {
		sourceTime := time.Now().Add(-2 * time.Minute)
		recvTime := sourceTime.Add(time.Minute)
		ackTime := recvTime.Add(time.Minute)
		recvHash, writeAckHash, ackHash := "0xrecv", "0xwriteack", "0xack"
		writeAckStatus := store.WriteAckStatusSuccess
		tr := NewTransfer(store.Packet{
			Status:                    store.RelayStatusDeliverAckPacket,
			SourceChainID:             "source",
			DestinationChainID:        "destination",
			SourceTxTime:              sourceTime,
			PacketSourceClientID:      "source-client",
			PacketDestinationClientID: "destination-client",
			PacketTimeoutTimestamp:    time.Now().Add(time.Hour),
			RecvTxHash:                &recvHash,
			RecvTxTime:                &recvTime,
			WriteAckTxHash:            &writeAckHash,
			WriteAckStatus:            &writeAckStatus,
			AckTxHash:                 &ackHash,
			AckTxTime:                 &ackTime,
		}, slog.Default())

		_, err := finisher.Process(ctx, tr)
		require.NoError(t, err)
		assert.Equal(t, uint64(2), metricCount("relays_completed_total"))

		_, err = finisher.Process(ctx, tr)
		require.NoError(t, err)
		assert.Equal(t, uint64(2), metricCount("relays_completed_total"))
	})

	t.Run("rejectedTerminalTransferIsNotCounted", func(t *testing.T) {
		recvHash, writeAckHash, ackHash := "0xrecv-error", "0xwriteack-error", "0xack-error"
		writeAckStatus := store.WriteAckStatusError
		tr := NewTransfer(store.Packet{
			Status:                 store.RelayStatusDeliverAckPacket,
			PacketTimeoutTimestamp: time.Now().Add(time.Hour),
			RecvTxHash:             &recvHash,
			WriteAckTxHash:         &writeAckHash,
			WriteAckStatus:         &writeAckStatus,
			AckTxHash:              &ackHash,
		}, slog.Default())

		_, err := finisher.Process(ctx, tr)
		require.NoError(t, err)
		assert.Equal(t, store.RelayStatusCompleteWithWriteAckError, tr.Status)
		assert.Equal(t, uint64(2), metricCount("relays_completed_total"))
	})
}
