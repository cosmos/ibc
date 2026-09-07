// SPDX-License-Identifier: Apache-2.0

package otel

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/metric"

	"github.com/cosmos/ibc/cli/internal/config"
)

func TestRegisterMetrics(t *testing.T) {
	t.Run("registersInstrumentation", func(t *testing.T) {
		// ARRANGE
		type instrumentation struct {
			Requests metric.Int64Counter
		}
		expected := &instrumentation{}

		// ACT
		actual := RegisterMetrics("test", func(meter metric.Meter) (*instrumentation, error) {
			counter, err := meter.Int64Counter("test_requests_total")
			require.NoError(t, err)
			expected.Requests = counter

			return expected, nil
		})

		// ASSERT
		require.Same(t, expected, actual)
		require.NotNil(t, actual.Requests)
		require.NotPanics(t, func() {
			actual.Requests.Add(context.Background(), 1)
		})
	})

	t.Run("panicsOnConstructionFailure", func(t *testing.T) {
		// ARRANGE
		type instrumentation struct{}
		constructor := func(metric.Meter) (*instrumentation, error) {
			return nil, errors.New("construction failed")
		}

		// ACT & ASSERT
		require.PanicsWithError(t,
			"failed to construct metrics for test: construction failed",
			func() {
				RegisterMetrics("test", constructor)
			},
		)
	})

	t.Run("panicsOnNilInstrumentation", func(t *testing.T) {
		// ARRANGE
		type instrumentation struct{}
		constructor := func(metric.Meter) (*instrumentation, error) {
			return nil, nil
		}

		// ACT & ASSERT
		require.PanicsWithError(t,
			"failed to construct metrics for test: constructor returned nil",
			func() {
				RegisterMetrics("test", constructor)
			},
		)
	})
}

func TestPrometheusMetrics(t *testing.T) {
	t.Run("servesRegisteredMetrics", func(t *testing.T) {
		// ARRANGE
		listenAddress := availableListenAddress(t)
		cfg := config.Observability{
			Type:          config.ObservabilitySimple,
			ListenAddress: listenAddress,
		}
		provider, err := New(t.Context(), cfg, testLogger())
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, provider.Stop())
		})

		counter, err := provider.meter.Meter("test").Int64Counter("requests",
			metric.WithDescription("Test request count"),
		)
		require.NoError(t, err)
		counter.Add(context.Background(), 1)

		// ACT
		client := &http.Client{Timeout: time.Second}
		resp, err := client.Get("http://" + listenAddress + simpleMetricsPath)
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, resp.Body.Close())
		})
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		// ASSERT
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Contains(t, string(body), "# HELP requests_total Test request count")
		assert.Contains(t, string(body), "requests_total")
	})
}

func TestDurationMilliseconds(t *testing.T) {
	t.Run("preservesFractionalMilliseconds", func(t *testing.T) {
		// ARRANGE
		duration := 1500 * time.Microsecond

		// ACT
		actual := durationMilliseconds(duration)

		// ASSERT
		assert.InDelta(t, 1.5, actual, 0.0001)
	})
}
