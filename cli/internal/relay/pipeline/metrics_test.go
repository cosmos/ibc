// SPDX-License-Identifier: Apache-2.0

package pipeline

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/cosmos/ibc/cli/internal/otel"
	"github.com/cosmos/ibc/cli/internal/relay/processors"
	"github.com/cosmos/ibc/cli/internal/store"
)

type metricBatchProcessor struct {
	process func([]*processors.Transfer) ([]*processors.Transfer, error)
}

func (*metricBatchProcessor) ShouldProcess(*processors.Transfer) bool { return true }

func (*metricBatchProcessor) Status() store.RelayStatus {
	return store.RelayStatusDeliverRecvPacket
}

func (p *metricBatchProcessor) Process(
	_ context.Context,
	batch []*processors.Transfer,
) ([]*processors.Transfer, error) {
	return p.process(batch)
}

func (*metricBatchProcessor) Cancel([]*processors.Transfer, error) {}

type batchMetricData struct {
	total metricdata.DataPoint[int64]
	size  metricdata.HistogramDataPoint[int64]
}

func TestPipelineMetrics(t *testing.T) {
	t.Run("batchProcessorMW", func(t *testing.T) {
		for _, tt := range []struct {
			name    string
			process func([]*processors.Transfer) ([]*processors.Transfer, error)
			result  string
		}{
			{
				name: "success",
				process: func(batch []*processors.Transfer) ([]*processors.Transfer, error) {
					return batch, nil
				},
				result: "ok",
			},
			{
				name: "topLevelError",
				process: func([]*processors.Transfer) ([]*processors.Transfer, error) {
					return nil, errors.New("boom")
				},
				result: "error",
			},
			{
				name: "mixedPerTransferFailure",
				process: func(batch []*processors.Transfer) ([]*processors.Transfer, error) {
					batch[0].ProcessingError = errors.New("poisoned")

					return batch, nil
				},
				result: "error",
			},
			{
				name: "allPoisonedOutputs",
				process: func(batch []*processors.Transfer) ([]*processors.Transfer, error) {
					for _, tr := range batch {
						tr.ProcessingError = errors.New("poisoned")
					}

					return batch, nil
				},
				result: "error",
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				// ARRANGE
				ctx := context.Background()
				reader := newTestMetrics(t)
				mw := NewBatchProcessorMW(
					&statusRecorder{},
					&metricBatchProcessor{process: tt.process},
				)

				// ACT
				_, err := mw.Process(ctx, []*processors.Transfer{testTransfer(t), testTransfer(t)})

				// ASSERT
				require.NoError(t, err)
				data := collectBatchMetrics(ctx, t, reader)

				expectedSizeAttributes := attribute.NewSet(
					otel.AttrChainID.String("8453"),
					otel.AttrProcessor.String("recv"),
				)
				assert.Equal(t, uint64(1), data.size.Count)
				assert.Equal(t, int64(2), data.size.Sum)
				assert.Equal(t, expectedSizeAttributes.ToSlice(), data.size.Attributes.ToSlice())

				expectedTotalAttributes := attribute.NewSet(
					otel.AttrChainID.String("8453"),
					otel.AttrProcessor.String("recv"),
					otel.AttrResult.String(tt.result),
				)
				assert.Equal(t, int64(1), data.total.Value)
				assert.Equal(t, expectedTotalAttributes.ToSlice(), data.total.Attributes.ToSlice())
			})
		}
	})

	t.Run("batchResultError", func(t *testing.T) {
		for _, tt := range []struct {
			name   string
			output []*processors.Transfer
		}{
			{name: "nilOutput", output: nil},
			{name: "emptyOutput", output: []*processors.Transfer{}},
			{name: "nilTransfer", output: []*processors.Transfer{nil}},
		} {
			t.Run(tt.name, func(t *testing.T) {
				// ARRANGE
				var processErr error

				// ACT
				err := batchResultError(processErr, tt.output)

				// ASSERT
				assert.NoError(t, err)
			})
		}
	})
}

func newTestMetrics(t *testing.T) *sdkmetric.ManualReader {
	t.Helper()

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

	return reader
}

func collectBatchMetrics(
	ctx context.Context,
	t *testing.T,
	reader *sdkmetric.ManualReader,
) batchMetricData {
	t.Helper()

	var resources metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(ctx, &resources))

	var data batchMetricData
	var foundTotal bool
	var foundSize bool

	for _, scope := range resources.ScopeMetrics {
		for _, metric := range scope.Metrics {
			switch metric.Name {
			case "batches_total":
				sum, ok := metric.Data.(metricdata.Sum[int64])
				require.True(t, ok)
				require.Len(t, sum.DataPoints, 1)
				data.total = sum.DataPoints[0]
				foundTotal = true
			case "batch_size":
				histogram, ok := metric.Data.(metricdata.Histogram[int64])
				require.True(t, ok)
				require.Len(t, histogram.DataPoints, 1)
				data.size = histogram.DataPoints[0]
				foundSize = true
			}
		}
	}

	require.True(t, foundTotal, "batches_total metric not found")
	require.True(t, foundSize, "batch_size metric not found")

	return data
}
