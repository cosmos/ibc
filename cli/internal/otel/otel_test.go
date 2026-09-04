// SPDX-License-Identifier: Apache-2.0

package otel

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/metric"

	"github.com/cosmos/ibc/cli/internal/config"
)

func TestProvider(t *testing.T) {
	t.Run("servesPrometheusMetrics", func(t *testing.T) {
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

	t.Run("rejectsUnsupportedType", func(t *testing.T) {
		// ARRANGE
		cfg := config.Observability{Type: config.ObservabilityOTEL}

		// ACT
		provider, err := New(t.Context(), cfg, testLogger())

		// ASSERT
		require.ErrorContains(t, err, "unsupported observability type")
		assert.Nil(t, provider)
	})

	t.Run("rejectsUnavailableListenAddress", func(t *testing.T) {
		// ARRANGE
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, ln.Close())
		})
		cfg := config.Observability{
			Type:          config.ObservabilitySimple,
			ListenAddress: ln.Addr().String(),
		}

		// ACT
		provider, err := New(t.Context(), cfg, testLogger())

		// ASSERT
		require.ErrorContains(t, err, "listenAddress")
		assert.Nil(t, provider)
	})
}

func availableListenAddress(t *testing.T) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := ln.Addr().String()
	require.NoError(t, ln.Close())

	return address
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
