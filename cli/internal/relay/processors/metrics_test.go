// SPDX-License-Identifier: Apache-2.0

package processors

import (
	"context"
	"log/slog"
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
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() {
		require.NoError(t, provider.Shutdown(context.Background()))
	})

	instruments, err := newInstrumentation(provider.Meter("test"))
	require.NoError(t, err)

	sourceTime := time.Now().Add(-2 * time.Minute)
	recvTime := sourceTime.Add(time.Minute)
	ackTime := recvTime.Add(time.Minute)
	tr := NewTransfer(store.Packet{
		SourceChainID:             "source",
		DestinationChainID:        "destination",
		SourceTxTime:              sourceTime,
		PacketSourceClientID:      "source-client",
		PacketDestinationClientID: "destination-client",
		RecvTxTime:                &recvTime,
		AckTxTime:                 &ackTime,
	}, slog.Default())
	tr.Status = store.RelayStatusCompleteWithAck

	ctx := context.Background()
	instruments.relayCompleted(ctx, tr)
	instruments.relayFinished(ctx, tr)
	instruments.transactionRetry(ctx, tr, relayTypeRecvToAck)

	var data metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(ctx, &data))

	counts := make(map[string]uint64)
	completedRelayTypes := make(map[string]bool)
	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			switch points := metric.Data.(type) {
			case metricdata.Sum[int64]:
				for _, point := range points.DataPoints {
					counts[metric.Name] += uint64(point.Value)
					if metric.Name == "relays_completed_total" {
						relayType, ok := point.Attributes.Value(otel.AttrRelayType)
						require.True(t, ok)
						completedRelayTypes[relayType.AsString()] = true
						assert.Equal(t, "source", attributeString(t, point.Attributes, otel.AttrChainID))
						assert.Equal(t, "destination", attributeString(t, point.Attributes, otel.AttrDestChainID))
					}
				}
			case metricdata.Histogram[float64]:
				for _, point := range points.DataPoints {
					counts[metric.Name] += point.Count
					assert.Len(t, point.Attributes.ToSlice(), 5)
				}
			}
		}
	}

	assert.Equal(t, uint64(2), counts["relays_completed_total"])
	assert.Equal(t, map[string]bool{
		string(relayTypeSendToRecv): true,
		string(relayTypeRecvToAck):  true,
	}, completedRelayTypes)
	assert.Equal(t, uint64(2), counts["relay_duration_seconds"])
	assert.Equal(t, uint64(1), counts["transaction_retries_total"])
}

func attributeString(
	t *testing.T,
	attributes attribute.Set,
	key attribute.Key,
) string {
	t.Helper()

	value, ok := attributes.Value(key)
	require.True(t, ok)

	return value.AsString()
}
