// SPDX-License-Identifier: Apache-2.0

package watcher

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/cosmos/ibc/cli/internal/otel"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

func TestSubscriptionHealth(t *testing.T) {
	db := watcherStore(t)
	reader := setupWatcherMetrics(t)
	synctest.Test(t, func(t *testing.T) {
		chain := newFakeChain(t)
		w := newTestWatcher(chain, db)
		chain.failNextSub(assert.AnError)
		require.Error(t, w.Start())
		assertSubscriptionUp(t, reader, 0)
		require.NoError(t, w.Start())
		defer func() { require.NoError(t, w.Stop()) }()
		assertSubscriptionUp(t, reader, 1)
		time.Sleep(time.Second)
		assertSubscriptionUp(t, reader, 1)

		chain.failNextSub(assert.AnError)
		chain.latestSub().errs <- assert.AnError
		synctest.Wait()
		assertSubscriptionUp(t, reader, 0)
		time.Sleep(DefaultMinBackoff)
		synctest.Wait()
		assertSubscriptionUp(t, reader, 0)
		time.Sleep(2 * DefaultMinBackoff)
		synctest.Wait()
		assertSubscriptionUp(t, reader, 1)
		require.NoError(t, w.Stop())
		assertSubscriptionUp(t, reader, 0)
	})
}

func assertSubscriptionUp(t *testing.T, reader *sdkmetric.ManualReader, want int64) {
	t.Helper()
	var data metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &data))
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name == "watcher_subscription_up" {
				gauge, ok := m.Data.(metricdata.Gauge[int64])
				require.True(t, ok)
				require.Len(t, gauge.DataPoints, 1)
				assert.Equal(t, want, gauge.DataPoints[0].Value)
				return
			}
		}
	}
	t.Fatal("subscription gauge missing")
}

func TestWatcherHandleEventMetrics(t *testing.T) {
	t.Run("acceptedEvent", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		reader := setupWatcherMetrics(t)
		w := newTestWatcher(newFakeChain(t), watcherStore(t))

		// ACT
		err := w.HandleEvent(ctx, sendPacketEvent(7))

		// ASSERT
		require.NoError(t, err)
		assert.Equal(t, int64(1), watcherEventCount(t, reader))
	})

	t.Run("removedEvent", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		reader := setupWatcherMetrics(t)
		w := newTestWatcher(newFakeChain(t), watcherStore(t))
		event := sendPacketEvent(7)
		event.Removed = true

		// ACT
		err := w.HandleEvent(ctx, event)

		// ASSERT
		require.NoError(t, err)
		assert.Equal(t, int64(0), watcherEventCount(t, reader))
	})

	t.Run("nonSendEvent", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		reader := setupWatcherMetrics(t)
		w := newTestWatcher(newFakeChain(t), watcherStore(t))
		event := sendPacketEvent(7)
		event.Kind = v2.KindWriteAck

		// ACT
		err := w.HandleEvent(ctx, event)

		// ASSERT
		require.NoError(t, err)
		assert.Equal(t, int64(0), watcherEventCount(t, reader))
	})

	t.Run("reEmittedEvent", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		reader := setupWatcherMetrics(t)
		w := newTestWatcher(newFakeChain(t), watcherStore(t))
		event := sendPacketEvent(7)

		// ACT
		acceptedErr := w.HandleEvent(ctx, event)
		event.Removed = true
		removedErr := w.HandleEvent(ctx, event)
		event.Removed = false
		reEmittedErr := w.HandleEvent(ctx, event)

		// ASSERT
		require.NoError(t, acceptedErr)
		require.NoError(t, removedErr)
		require.NoError(t, reEmittedErr)
		assert.Equal(t, int64(2), watcherEventCount(t, reader))
	})

	t.Run("storageError", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		reader := setupWatcherMetrics(t)
		w := newTestWatcher(newFakeChain(t), &failingStore{
			ClearStore: watcherStore(t),
			err:        errors.New("store unavailable"),
		})

		// ACT
		err := w.HandleEvent(ctx, sendPacketEvent(8))

		// ASSERT
		require.ErrorContains(t, err, "store unavailable")
		assert.Equal(t, int64(1), watcherEventCount(t, reader))
	})
}

func setupWatcherMetrics(t *testing.T) *sdkmetric.ManualReader {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() {
		require.NoError(t, provider.Shutdown(context.Background()))
	})

	instruments, err := newInstrumentation(provider.Meter("test"))
	require.NoError(t, err)

	previousMetrics := metrics
	metrics = instruments
	t.Cleanup(func() {
		metrics = previousMetrics
	})

	return reader
}

func watcherEventCount(t *testing.T, reader *sdkmetric.ManualReader) int64 {
	t.Helper()

	var data metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &data))

	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != "watcher_events_total" {
				continue
			}

			sum, ok := metric.Data.(metricdata.Sum[int64])
			require.True(t, ok)
			require.Len(t, sum.DataPoints, 1)

			point := sum.DataPoints[0]
			chainID, ok := point.Attributes.Value(otel.AttrChainID)
			require.True(t, ok)
			assert.Equal(t, sourceChainID, chainID.AsString())

			eventType, ok := point.Attributes.Value(otel.AttrType)
			require.True(t, ok)
			assert.Equal(t, "send_packet", eventType.AsString())

			return point.Value
		}
	}

	return 0
}
