// SPDX-License-Identifier: Apache-2.0

package dispatch

import (
	"context"
	"log/slog"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/cosmos/ibc/cli/internal/store"
)

type heartbeatStore struct {
	packets            []store.Packet
	listErr, updateErr error
	gate               <-chan struct{}
}

func (s *heartbeatStore) ListDispatchablePackets(ctx context.Context) ([]store.Packet, error) {
	if s.gate != nil {
		select {
		case <-s.gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return s.packets, s.listErr
}

func (s *heartbeatStore) UpdatePacketStatus(context.Context, store.PacketKey, store.RelayStatus) error {
	return s.updateErr
}

func TestDispatcherHeartbeat(t *testing.T) {
	instruments, reader := newTestInstrumentation(t)
	previous := metrics
	metrics = instruments
	t.Cleanup(func() { metrics = previous })
	synctest.Test(t, func(t *testing.T) {
		storage := &heartbeatStore{}
		d := NewRelayDispatcher(storage, &fakePipelines{err: assert.AnError}, time.Second, slog.Default())
		ctx := context.Background()
		require.NoError(t, d.SubmitWaitingDispatchablePackets(ctx))
		first := dispatcherHeartbeat(t, reader)
		time.Sleep(time.Second)
		require.NoError(t, d.SubmitWaitingDispatchablePackets(ctx))
		last := dispatcherHeartbeat(t, reader)
		assert.Greater(t, last, first, "empty successful polls must advance the heartbeat")

		storage.listErr = assert.AnError
		require.ErrorIs(t, d.SubmitWaitingDispatchablePackets(ctx), assert.AnError)
		assert.InDelta(t, last, dispatcherHeartbeat(t, reader), 0)
		storage.listErr = nil
		storage.packets = []store.Packet{packetForRoute(testRoute)}
		storage.updateErr = assert.AnError
		require.ErrorIs(t, d.SubmitWaitingDispatchablePackets(ctx), assert.AnError)
		assert.InDelta(t, last, dispatcherHeartbeat(t, reader), 0)

		storage.packets = nil
		gate := make(chan struct{})
		storage.gate = gate
		done := make(chan error, 1)
		go func() { done <- d.SubmitWaitingDispatchablePackets(ctx) }()
		synctest.Wait()
		time.Sleep(time.Second)
		assert.InDelta(t, last, dispatcherHeartbeat(t, reader), 0, "blocked polls must not advance the heartbeat")
		close(gate)
		require.NoError(t, <-done)
		assert.Greater(t, dispatcherHeartbeat(t, reader), last)
	})
}

func dispatcherHeartbeat(t *testing.T, reader *sdkmetric.ManualReader) float64 {
	t.Helper()
	var data metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &data))
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name == "dispatcher_last_success_timestamp_seconds" {
				gauge, ok := m.Data.(metricdata.Gauge[float64])
				require.True(t, ok)
				require.Len(t, gauge.DataPoints, 1)
				return gauge.DataPoints[0].Value
			}
		}
	}
	t.Fatal("dispatcher heartbeat missing")
	return 0
}
