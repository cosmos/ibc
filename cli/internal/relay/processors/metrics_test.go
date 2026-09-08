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

type metricsTestSuite struct {
	ctx         context.Context
	reader      *sdkmetric.ManualReader
	instruments *instrumentation
}

func newMetricsTestSuite(t *testing.T) *metricsTestSuite {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() {
		require.NoError(t, provider.Shutdown(context.Background()))
	})

	instruments, err := newInstrumentation(provider.Meter("test"))
	require.NoError(t, err)

	return &metricsTestSuite{
		ctx:         context.Background(),
		reader:      reader,
		instruments: instruments,
	}
}

func (s *metricsTestSuite) install(t *testing.T) {
	t.Helper()

	originalMetrics := metrics
	metrics = s.instruments
	t.Cleanup(func() {
		metrics = originalMetrics
	})
}

func TestMetrics(t *testing.T) {
	t.Run("completedRelayLegs", func(t *testing.T) {
		sourceTime := time.Unix(1_000, 0)
		recvTime := sourceTime.Add(30 * time.Second)
		ackTime := recvTime.Add(45 * time.Second)
		timeoutTime := sourceTime.Add(90 * time.Second)

		for _, tt := range []struct {
			name     string
			status   store.RelayStatus
			expected map[relayType]float64
		}{
			{
				name:   "ack",
				status: store.RelayStatusCompleteWithAck,
				expected: map[relayType]float64{
					relayTypeSendToRecv: 30,
					relayTypeRecvToAck:  45,
				},
			},
			{
				name:   "timeout",
				status: store.RelayStatusCompleteWithTimeout,
				expected: map[relayType]float64{
					relayTypeSendToTimeout: 90,
				},
			},
			{
				name:     "writeAckError",
				status:   store.RelayStatusCompleteWithWriteAckError,
				expected: nil,
			},
			{
				name:     "unknownStatus",
				status:   store.RelayStatus("UNKNOWN"),
				expected: nil,
			},
			{
				name:     "nonTerminal",
				status:   store.RelayStatusDeliverAckPacket,
				expected: nil,
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				// ARRANGE
				suite := newMetricsTestSuite(t)
				tr := NewTransfer(store.Packet{
					SourceChainID:             "source",
					DestinationChainID:        "destination",
					SourceTxTime:              sourceTime,
					PacketSourceClientID:      "source-client",
					PacketDestinationClientID: "destination-client",
					RecvTxTime:                &recvTime,
					AckTxTime:                 &ackTime,
					TimeoutTxTime:             &timeoutTime,
				}, slog.Default())
				tr.Status = tt.status

				// ACT
				suite.instruments.relayCompleted(suite.ctx, tr)

				// ASSERT
				assertCompletionMetrics(t, suite, tr, tt.expected)
			})
		}
	})

	t.Run("transactionRetry", func(t *testing.T) {
		// ARRANGE
		suite := newMetricsTestSuite(t)
		tr := NewTransfer(store.Packet{
			SourceChainID:             "source",
			DestinationChainID:        "destination",
			PacketSourceClientID:      "source-client",
			PacketDestinationClientID: "destination-client",
		}, slog.Default())

		// ACT
		suite.instruments.transactionRetry(suite.ctx, tr, relayTypeRecvToAck)

		// ASSERT
		var data metricdata.ResourceMetrics
		require.NoError(t, suite.reader.Collect(suite.ctx, &data))

		for _, scope := range data.ScopeMetrics {
			for _, metric := range scope.Metrics {
				if metric.Name != "transaction_retries_total" {
					continue
				}

				sum, ok := metric.Data.(metricdata.Sum[int64])
				require.True(t, ok)
				require.Len(t, sum.DataPoints, 1)
				assert.Equal(t, int64(1), sum.DataPoints[0].Value)

				expectedAttributes := attribute.NewSet(
					otel.AttrChainID.String("source"),
					otel.AttrDestChainID.String("destination"),
					otel.AttrClientID.String("source-client"),
					otel.AttrDestClientID.String("destination-client"),
					otel.AttrType.String(string(relayTypeRecvToAck)),
				)
				assert.Equal(t, expectedAttributes.ToSlice(), sum.DataPoints[0].Attributes.ToSlice())

				return
			}
		}

		require.Fail(t, "transaction_retries_total was not collected")
	})

	t.Run("stateFinisherEmission", func(t *testing.T) {
		t.Run("retryAndNonTerminalTransferIsNotCounted", func(t *testing.T) {
			// ARRANGE
			suite := newMetricsTestSuite(t)
			suite.install(t)
			tr := NewTransfer(store.Packet{
				Status:                 store.RelayStatusDeliverRecvPacket,
				PacketTimeoutTimestamp: time.Now().Add(time.Hour),
			}, slog.Default())
			tr.ProcessingError = ErrRetryingRecvPacket

			// ACT
			_, err := NewStateFinisher(&fakeStatusStorage{}).Process(suite.ctx, tr)

			// ASSERT
			require.NoError(t, err)
			assertCompletionMetrics(t, suite, tr, nil)
		})

		t.Run("failedTerminalPersistenceIsNotCounted", func(t *testing.T) {
			// ARRANGE
			suite := newMetricsTestSuite(t)
			suite.install(t)
			timeoutHash := "0xtimeout-error"
			tr := NewTransfer(store.Packet{
				Status:                 store.RelayStatusDeliverTimeoutPacket,
				PacketTimeoutTimestamp: time.Now().Add(-time.Hour),
				TimeoutTxHash:          &timeoutHash,
			}, slog.Default())

			// ACT
			_, err := NewStateFinisher(&fakeStatusStorage{err: assert.AnError}).Process(suite.ctx, tr)

			// ASSERT
			require.NoError(t, err)
			require.ErrorIs(t, tr.ProcessingError, assert.AnError)
			assert.Equal(t, store.RelayStatusDeliverTimeoutPacket, tr.Status)
			assertCompletionMetrics(t, suite, tr, nil)
		})

		t.Run("successfulTerminalTransferIsCountedOnce", func(t *testing.T) {
			// ARRANGE
			suite := newMetricsTestSuite(t)
			suite.install(t)
			sourceTime := time.Unix(1_000, 0)
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
			finisher := NewStateFinisher(&fakeStatusStorage{})

			// ACT
			_, err := finisher.Process(suite.ctx, tr)

			// ASSERT
			require.NoError(t, err)
			assertCompletionMetrics(t, suite, tr, map[relayType]float64{
				relayTypeSendToRecv: 60,
				relayTypeRecvToAck:  60,
			})

			// ACT #2
			_, err = finisher.Process(suite.ctx, tr)

			// ASSERT #2
			require.NoError(t, err)
			assertCompletionMetrics(t, suite, tr, map[relayType]float64{
				relayTypeSendToRecv: 60,
				relayTypeRecvToAck:  60,
			})
		})

		t.Run("writeAckErrorIsNotCounted", func(t *testing.T) {
			// ARRANGE
			suite := newMetricsTestSuite(t)
			suite.install(t)
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

			// ACT
			_, err := NewStateFinisher(&fakeStatusStorage{}).Process(suite.ctx, tr)

			// ASSERT
			require.NoError(t, err)
			assert.Equal(t, store.RelayStatusCompleteWithWriteAckError, tr.Status)
			assertCompletionMetrics(t, suite, tr, nil)
		})
	})
}

func assertCompletionMetrics(
	t *testing.T,
	suite *metricsTestSuite,
	tr *Transfer,
	expected map[relayType]float64,
) {
	t.Helper()

	completed, durations := suite.collectCompletionMetrics(t)
	require.Len(t, completed, len(expected))
	require.Len(t, durations, len(expected))

	for kind, expectedDuration := range expected {
		expectedAttributes := completionAttributes(tr, kind)

		count, ok := completed[kind]
		require.True(t, ok)
		assert.Equal(t, int64(1), count.Value)
		assert.Equal(t, expectedAttributes.ToSlice(), count.Attributes.ToSlice())

		duration, ok := durations[kind]
		require.True(t, ok)
		assert.Equal(t, uint64(1), duration.Count)
		assert.InDelta(t, expectedDuration, duration.Sum, 0)
		assert.Equal(t, expectedAttributes.ToSlice(), duration.Attributes.ToSlice())
	}
}

func (s *metricsTestSuite) collectCompletionMetrics(t *testing.T) (
	map[relayType]metricdata.DataPoint[int64],
	map[relayType]metricdata.HistogramDataPoint[float64],
) {
	t.Helper()

	var data metricdata.ResourceMetrics
	require.NoError(t, s.reader.Collect(s.ctx, &data))

	completed := make(map[relayType]metricdata.DataPoint[int64])
	durations := make(map[relayType]metricdata.HistogramDataPoint[float64])
	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			switch points := metric.Data.(type) {
			case metricdata.Sum[int64]:
				if metric.Name != "relays_completed_total" {
					continue
				}
				for _, point := range points.DataPoints {
					completed[relayType(attributeString(t, point.Attributes, otel.AttrType))] = point
				}
			case metricdata.Histogram[float64]:
				if metric.Name != "relay_duration_seconds" {
					continue
				}
				for _, point := range points.DataPoints {
					durations[relayType(attributeString(t, point.Attributes, otel.AttrType))] = point
				}
			}
		}
	}

	return completed, durations
}

func completionAttributes(tr *Transfer, kind relayType) attribute.Set {
	return attribute.NewSet(
		otel.AttrChainID.String(tr.SourceChainID),
		otel.AttrDestChainID.String(tr.DestinationChainID),
		otel.AttrClientID.String(tr.PacketSourceClientID),
		otel.AttrDestClientID.String(tr.PacketDestinationClientID),
		otel.AttrType.String(string(kind)),
	)
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
