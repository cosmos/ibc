// SPDX-License-Identifier: Apache-2.0

package dispatch

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/cosmos/ibc/cli/internal/otel"
	"github.com/cosmos/ibc/cli/internal/store"
)

func TestInstrumentation(t *testing.T) {
	t.Run("recordsPacketsByRoute", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		instruments, reader := newTestInstrumentation(t)
		routeA := routeKey{
			chainID:      "source-a",
			destChainID:  "destination-a",
			clientID:     "client-a",
			destClientID: "destination-client-a",
		}
		routeB := routeKey{
			chainID:      "source-b",
			destChainID:  "destination-b",
			clientID:     "client-b",
			destClientID: "destination-client-b",
		}

		// ACT
		instruments.packetsPending(ctx, []store.Packet{
			packetForRoute(routeA),
			packetForRoute(routeA),
			packetForRoute(routeB),
		})

		// ASSERT
		points := collectPacketsPending(ctx, t, reader)
		require.Len(t, points, 2)
		assert.Equal(t, int64(2), points[routeA])
		assert.Equal(t, int64(1), points[routeB])
	})

	t.Run("recordsZeroForRouteWithoutPendingPackets", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		instruments, reader := newTestInstrumentation(t)
		routeA := routeKey{
			chainID:      "source-a",
			destChainID:  "destination-a",
			clientID:     "client-a",
			destClientID: "destination-client-a",
		}
		routeB := routeKey{
			chainID:      "source-b",
			destChainID:  "destination-b",
			clientID:     "client-b",
			destClientID: "destination-client-b",
		}
		instruments.packetsPending(ctx, []store.Packet{packetForRoute(routeA)})

		// ACT
		instruments.packetsPending(ctx, []store.Packet{packetForRoute(routeB)})

		// ASSERT
		points := collectPacketsPending(ctx, t, reader)
		require.Len(t, points, 2)
		assert.Zero(t, points[routeA])
		assert.Equal(t, int64(1), points[routeB])
	})

	t.Run("serializesConcurrentRecordings", func(t *testing.T) {
		// ARRANGE
		const routeCount = 32

		ctx := context.Background()
		instruments, reader := newTestInstrumentation(t)
		var wg sync.WaitGroup
		wg.Add(routeCount)

		// ACT
		for i := range routeCount {
			go func() {
				defer wg.Done()

				route := routeKey{
					chainID:      fmt.Sprintf("source-%d", i),
					destChainID:  fmt.Sprintf("destination-%d", i),
					clientID:     fmt.Sprintf("client-%d", i),
					destClientID: fmt.Sprintf("destination-client-%d", i),
				}
				instruments.packetsPending(ctx, []store.Packet{packetForRoute(route)})
			}()
		}
		wg.Wait()

		// ASSERT
		points := collectPacketsPending(ctx, t, reader)
		require.Len(t, points, routeCount)

		var total int64
		for _, value := range points {
			total += value
		}
		assert.Equal(t, int64(1), total)
		assert.Len(t, instruments.routesFromPrevCall, 1)
	})
}

func TestExcessiveRelayLatency(t *testing.T) {
	now := time.Now()

	base := store.Packet{
		Status:                    store.RelayStatusPending,
		SourceChainID:             "source",
		DestinationChainID:        "dest",
		PacketSourceClientID:      "client",
		PacketDestinationClientID: "dest-client",
	}

	t.Run("sendToRecv", func(t *testing.T) {
		ctx := context.Background()

		t.Run("pastThreshold", func(t *testing.T) {
			instruments, reader := newTestInstrumentation(t)
			packet := base
			packet.SourceTxTime = now.Add(-excessiveRelayLatency - time.Minute)
			packet.PacketTimeoutTimestamp = now.Add(time.Hour)

			instruments.excessiveRelayLatency(ctx, []store.Packet{packet})

			points := collectRelayLatency(ctx, t, reader)
			require.Len(t, points, 1)
			point := points[0]
			assert.Equal(t, int64(1), point.value)
			assert.Equal(t, legSendToRecv, attributeValue(t, point.attributes, otel.AttrType))
			assert.Equal(t, "source", attributeValue(t, point.attributes, otel.AttrChainID))
			assert.Equal(t, "dest", attributeValue(t, point.attributes, otel.AttrDestChainID))
			assert.Equal(t, "client", attributeValue(t, point.attributes, otel.AttrClientID))
			assert.Equal(t, "dest-client", attributeValue(t, point.attributes, otel.AttrDestClientID))
		})

		t.Run("withinThreshold", func(t *testing.T) {
			instruments, reader := newTestInstrumentation(t)
			packet := base
			packet.SourceTxTime = now.Add(-time.Minute)
			packet.PacketTimeoutTimestamp = now.Add(time.Hour)

			instruments.excessiveRelayLatency(ctx, []store.Packet{packet})

			assert.Empty(t, collectRelayLatency(ctx, t, reader))
		})
	})

	t.Run("recvToAck", func(t *testing.T) {
		ctx := context.Background()
		instruments, reader := newTestInstrumentation(t)

		writeAckHash := "0xwriteack"
		writeAckTime := now.Add(-excessiveRelayLatency - time.Minute)
		packet := base
		packet.WriteAckTxHash = &writeAckHash
		packet.WriteAckTxTime = &writeAckTime
		packet.PacketTimeoutTimestamp = now.Add(time.Hour)

		instruments.excessiveRelayLatency(ctx, []store.Packet{packet})

		points := collectRelayLatency(ctx, t, reader)
		require.Len(t, points, 1)
		assert.Equal(t, legRecvToAck, attributeValue(t, points[0].attributes, otel.AttrType))
	})

	t.Run("sendToTimeout", func(t *testing.T) {
		ctx := context.Background()

		t.Run("pastTimeoutPlusDelay", func(t *testing.T) {
			instruments, reader := newTestInstrumentation(t)
			packet := base
			packet.PacketTimeoutTimestamp = now.Add(-timeoutExcessiveDelay - time.Minute)
			packet.SourceTxTime = now.Add(-time.Hour)

			instruments.excessiveRelayLatency(ctx, []store.Packet{packet})

			points := collectRelayLatency(ctx, t, reader)
			require.Len(t, points, 1)
			assert.Equal(t, legSendToTimeout, attributeValue(t, points[0].attributes, otel.AttrType))
		})

		t.Run("withinTimeoutDelay", func(t *testing.T) {
			instruments, reader := newTestInstrumentation(t)
			packet := base
			packet.PacketTimeoutTimestamp = now.Add(-time.Minute)
			packet.SourceTxTime = now.Add(-time.Hour)

			instruments.excessiveRelayLatency(ctx, []store.Packet{packet})

			assert.Empty(t, collectRelayLatency(ctx, t, reader))
		})

		t.Run("waitsForSourceFinality", func(t *testing.T) {
			instruments, reader := newTestInstrumentation(t)
			packet := base
			packet.PacketTimeoutTimestamp = now.Add(-timeoutExcessiveDelay - time.Minute)
			packet.SourceTxTime = now.Add(-sourceFinalityDelay + time.Minute)

			instruments.excessiveRelayLatency(ctx, []store.Packet{packet})

			assert.Empty(t, collectRelayLatency(ctx, t, reader))
		})
	})

	t.Run("terminalStatusIsSkipped", func(t *testing.T) {
		ctx := context.Background()
		instruments, reader := newTestInstrumentation(t)
		packet := base
		packet.Status = store.RelayStatusCompleteWithAck
		packet.SourceTxTime = now.Add(-time.Hour)
		packet.PacketTimeoutTimestamp = now.Add(time.Hour)

		instruments.excessiveRelayLatency(ctx, []store.Packet{packet})

		assert.Empty(t, collectRelayLatency(ctx, t, reader))
	})
}

func collectRelayLatency(
	ctx context.Context,
	t *testing.T,
	reader *sdkmetric.ManualReader,
) []relayLatencyPoint {
	t.Helper()

	var data metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(ctx, &data))

	var points []relayLatencyPoint
	for _, scope := range data.ScopeMetrics {
		for _, collectedMetric := range scope.Metrics {
			if collectedMetric.Name != "excessive_relay_latency_total" {
				continue
			}

			sum, ok := collectedMetric.Data.(metricdata.Sum[int64])
			require.True(t, ok)
			for _, point := range sum.DataPoints {
				points = append(points, relayLatencyPoint{value: point.Value, attributes: point.Attributes})
			}
		}
	}

	return points
}

type relayLatencyPoint struct {
	value      int64
	attributes attribute.Set
}

func newTestInstrumentation(t *testing.T) (*instrumentation, *sdkmetric.ManualReader) {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() {
		require.NoError(t, provider.Shutdown(context.Background()))
	})

	instruments, err := newInstrumentation(provider.Meter("test"))
	require.NoError(t, err)

	return instruments, reader
}

func packetForRoute(route routeKey) store.Packet {
	return store.Packet{
		SourceChainID:             route.chainID,
		DestinationChainID:        route.destChainID,
		PacketSourceClientID:      route.clientID,
		PacketDestinationClientID: route.destClientID,
	}
}

func collectPacketsPending(
	ctx context.Context,
	t *testing.T,
	reader *sdkmetric.ManualReader,
) map[routeKey]int64 {
	t.Helper()

	var data metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(ctx, &data))

	pointsByRoute := make(map[routeKey]int64)
	for _, scope := range data.ScopeMetrics {
		for _, collectedMetric := range scope.Metrics {
			if collectedMetric.Name != "packets_pending" {
				continue
			}

			gauge, ok := collectedMetric.Data.(metricdata.Gauge[int64])
			require.True(t, ok)
			for _, point := range gauge.DataPoints {
				route := routeKey{
					chainID:      attributeValue(t, point.Attributes, otel.AttrChainID),
					destChainID:  attributeValue(t, point.Attributes, otel.AttrDestChainID),
					clientID:     attributeValue(t, point.Attributes, otel.AttrClientID),
					destClientID: attributeValue(t, point.Attributes, otel.AttrDestClientID),
				}
				pointsByRoute[route] = point.Value
			}

			return pointsByRoute
		}
	}

	require.Fail(t, "packets_pending was not collected")

	return nil
}

func attributeValue(t *testing.T, attributes attribute.Set, key attribute.Key) string {
	t.Helper()

	value, ok := attributes.Value(key)
	require.True(t, ok)

	return value.AsString()
}
