// SPDX-License-Identifier: Apache-2.0

package signer

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/cosmos/ibc/cli/internal/config"
)

func TestMetrics(t *testing.T) {
	t.Run("instrumentedSignerRecordsSign", func(t *testing.T) {
		// ARRANGE
		reader := setupTestMetrics(t)
		base, err := GenerateLocalEd25519Signer()
		require.NoError(t, err)
		signer := metricsWrapper(base, "alice")

		// ACT
		signature, err := signer.Sign(t.Context(), []byte("message"))

		// ASSERT
		require.NoError(t, err)
		assert.NotEmpty(t, signature)
		assert.Equal(t, base.PublicKey(), signer.PublicKey())

		var data metricdata.ResourceMetrics
		require.NoError(t, reader.Collect(t.Context(), &data))
		require.Len(t, data.ScopeMetrics, 1)
		require.Len(t, data.ScopeMetrics[0].Metrics, 1)

		histogram, ok := data.ScopeMetrics[0].Metrics[0].Data.(metricdata.Histogram[float64])
		require.True(t, ok)
		require.Len(t, histogram.DataPoints, 1)
		assert.Equal(t, uint64(1), histogram.DataPoints[0].Count)
		assert.ElementsMatch(t, []attribute.KeyValue{
			attribute.String("operation", "sign"),
			attribute.String("signer", "alice"),
			attribute.String("type", typeLocalEDDSA),
			attribute.String("result", "ok"),
		}, histogram.DataPoints[0].Attributes.ToSlice())
	})

	t.Run("configuredLocalSignerHasMetricMetadata", func(t *testing.T) {
		// ARRANGE
		home := t.TempDir()
		t.Setenv("HOME", home)

		key, err := GenerateLocalEd25519Signer()
		require.NoError(t, err)

		path := filepath.Join(home, "keys", "signer.json")
		require.NoError(t, key.StoreToFile(path))

		// ACT
		loadedSigner, _, err := NewSignerFromConfig(t.Context(), config.SignerConfig{
			Alias: "local",
			Type:  config.SignerLocal,
			File:  "~/keys/signer.json",
		})

		// ASSERT
		require.NoError(t, err)
		instrumented, ok := loadedSigner.(*instrumentedSigner)
		require.True(t, ok)
		assert.Equal(t, "local", instrumented.alias)
		assert.Equal(t, typeLocalEDDSA, instrumented.keyType)
	})
}

func setupTestMetrics(t *testing.T) *sdkmetric.ManualReader {
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
