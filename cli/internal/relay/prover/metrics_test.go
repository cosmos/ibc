// SPDX-License-Identifier: Apache-2.0

package prover

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	"github.com/cosmos/ibc/cli/internal/otel"
	"github.com/cosmos/ibc/cli/internal/tests/mocks"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

func TestInstrumentation(t *testing.T) {
	t.Run("latestProvableHeightRecordsOperationAndHeight", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		reader := setTestMetrics(t)
		prover := mocks.NewMockProver(t)
		timestamp := time.Unix(1_000, 0)
		prover.EXPECT().LatestProvableHeight(ctx).Return(uint64(42), timestamp, nil).Once()
		instrumented := metricsWrapper("chain-a", "client-0", "attestation", prover)

		// ACT
		height, actualTimestamp, err := instrumented.LatestProvableHeight(ctx)

		// ASSERT
		require.NoError(t, err)
		assert.Equal(t, uint64(42), height)
		assert.Equal(t, timestamp, actualTimestamp)

		collected := collectMetrics(ctx, t, reader)
		operation := requireFloat64Histogram(t, collected, "prover_operation")
		require.Len(t, operation.DataPoints, 1)
		assert.Equal(t, uint64(1), operation.DataPoints[0].Count)
		assert.GreaterOrEqual(t, operation.DataPoints[0].Sum, float64(0))
		expectedOperationAttributes := operationAttributes("latest_provable_height", "ok")
		assert.Equal(t, expectedOperationAttributes.ToSlice(), operation.DataPoints[0].Attributes.ToSlice())

		latestHeight := requireInt64Gauge(t, collected, "latest_provable_height")
		require.Len(t, latestHeight.DataPoints, 1)
		assert.Equal(t, int64(42), latestHeight.DataPoints[0].Value)
		expectedHeightAttributes := baseAttributes()
		assert.Equal(t, expectedHeightAttributes.ToSlice(), latestHeight.DataPoints[0].Attributes.ToSlice())
	})

	t.Run("stateProofRecordsFailedOperation", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		reader := setTestMetrics(t)
		prover := mocks.NewMockProver(t)
		prover.EXPECT().StateProof(ctx, uint64(7)).Return(nil, errors.New("proof unavailable")).Once()
		instrumented := metricsWrapper("chain-a", "client-0", "attestation", prover)

		// ACT
		proof, err := instrumented.StateProof(ctx, 7)

		// ASSERT
		require.ErrorContains(t, err, "proof unavailable")
		assert.Nil(t, proof)

		collected := collectMetrics(ctx, t, reader)
		operation := requireFloat64Histogram(t, collected, "prover_operation")
		require.Len(t, operation.DataPoints, 1)
		assert.Equal(t, uint64(1), operation.DataPoints[0].Count)
		expectedAttributes := operationAttributes("state_proof", "error")
		assert.Equal(t, expectedAttributes.ToSlice(), operation.DataPoints[0].Attributes.ToSlice())
	})

	t.Run("packetProofsRecordsOperationAndBatchSize", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		reader := setTestMetrics(t)
		prover := mocks.NewMockProver(t)
		packets := []channeltypesv2.Packet{{Sequence: 1}, {Sequence: 2}}
		expectedProofs := [][]byte{{0x1}, {0x2}}
		prover.EXPECT().
			PacketProofs(ctx, uint64(9), v2.ProofKindPacketCommitment, packets).
			Return(expectedProofs, nil).
			Once()
		instrumented := metricsWrapper("chain-a", "client-0", "attestation", prover)

		// ACT
		proofs, err := instrumented.PacketProofs(ctx, 9, v2.ProofKindPacketCommitment, packets)

		// ASSERT
		require.NoError(t, err)
		assert.Equal(t, expectedProofs, proofs)

		collected := collectMetrics(ctx, t, reader)
		expectedAttributes := attribute.NewSet(
			otel.AttrChainID.String("chain-a"),
			otel.AttrClientID.String("client-0"),
			otel.AttrType.String("attestation"),
			otel.AttrProofKind.String("packet_commitment"),
		)

		operation := requireFloat64Histogram(t, collected, "prover_operation")
		require.Len(t, operation.DataPoints, 1)
		assert.Equal(t, uint64(1), operation.DataPoints[0].Count)
		expectedOperationAttributes := attribute.NewSet(
			append(expectedAttributes.ToSlice(),
				otel.AttrResult.String("ok"),
				otel.AttrOp.String("packet_proofs"),
			)...,
		)
		assert.Equal(t, expectedOperationAttributes.ToSlice(), operation.DataPoints[0].Attributes.ToSlice())

		batchSize := requireInt64Histogram(t, collected, "packet_batch_size")
		require.Len(t, batchSize.DataPoints, 1)
		assert.Equal(t, uint64(1), batchSize.DataPoints[0].Count)
		assert.Equal(t, int64(2), batchSize.DataPoints[0].Sum)
		assert.Equal(t, expectedAttributes.ToSlice(), batchSize.DataPoints[0].Attributes.ToSlice())
	})
}

func setTestMetrics(t *testing.T) *sdkmetric.ManualReader {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	instruments, err := newInstrumentation(provider.Meter("test"))
	require.NoError(t, err)

	previousMetrics := metrics
	metrics = instruments
	t.Cleanup(func() {
		metrics = previousMetrics
		require.NoError(t, provider.Shutdown(context.Background()))
	})

	return reader
}

func collectMetrics(ctx context.Context, t *testing.T, reader *sdkmetric.ManualReader) map[string]metricdata.Metrics {
	t.Helper()

	var data metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(ctx, &data))

	collected := make(map[string]metricdata.Metrics)
	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			collected[metric.Name] = metric
		}
	}

	return collected
}

func requireFloat64Histogram(
	t *testing.T,
	collected map[string]metricdata.Metrics,
	name string,
) metricdata.Histogram[float64] {
	t.Helper()

	metric, ok := collected[name]
	require.True(t, ok)
	histogram, ok := metric.Data.(metricdata.Histogram[float64])
	require.True(t, ok)

	return histogram
}

func requireInt64Histogram(
	t *testing.T,
	collected map[string]metricdata.Metrics,
	name string,
) metricdata.Histogram[int64] {
	t.Helper()

	metric, ok := collected[name]
	require.True(t, ok)
	histogram, ok := metric.Data.(metricdata.Histogram[int64])
	require.True(t, ok)

	return histogram
}

func requireInt64Gauge(
	t *testing.T,
	collected map[string]metricdata.Metrics,
	name string,
) metricdata.Gauge[int64] {
	t.Helper()

	metric, ok := collected[name]
	require.True(t, ok)
	gauge, ok := metric.Data.(metricdata.Gauge[int64])
	require.True(t, ok)

	return gauge
}

func baseAttributes() attribute.Set {
	return attribute.NewSet(
		otel.AttrChainID.String("chain-a"),
		otel.AttrClientID.String("client-0"),
	)
}

func operationAttributes(operation, result string) attribute.Set {
	return attribute.NewSet(
		otel.AttrChainID.String("chain-a"),
		otel.AttrClientID.String("client-0"),
		otel.AttrType.String("attestation"),
		otel.AttrResult.String(result),
		otel.AttrOp.String(operation),
	)
}
