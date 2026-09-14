// SPDX-License-Identifier: Apache-2.0

package otel

import (
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/contrib/otelconf"

	"github.com/cosmos/ibc/cli/internal/config"
)

func TestProvider(t *testing.T) {
	t.Run("rejectsUnsupportedType", func(t *testing.T) {
		// ARRANGE
		cfg := config.Observability{Type: "unsupported"}

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
			Type:                    config.ObservabilitySimple,
			SimpleMetricsListenAddr: ln.Addr().String(),
		}

		// ACT
		provider, err := New(t.Context(), cfg, testLogger())

		// ASSERT
		require.ErrorContains(t, err, "simpleMetricsListenAddr")
		assert.Nil(t, provider)
	})
}

func TestOtelFileMeterProvider(t *testing.T) {
	t.Run("rejectsMalformedConfig", func(t *testing.T) {
		// ARRANGE
		path := writeOtelConfig(t, "meter_provider: [")
		cfg := config.Observability{Type: config.ObservabilityOTEL, OtelFile: path}

		// ACT
		meterProvider, stop, err := newOtelFileMeterProvider(cfg, testLogger())

		// ASSERT
		require.ErrorContains(t, err, "parse OTEL config file")
		assert.Nil(t, meterProvider)
		assert.Nil(t, stop)
	})

	t.Run("setsUpAndShutsDownSDK", func(t *testing.T) {
		// ARRANGE
		path := writeOtelConfig(t, "file_format: \"1.0-rc.2\"\nmeter_provider: {}\n")
		cfg := config.Observability{Type: config.ObservabilityOTEL, OtelFile: path}

		// ACT
		meterProvider, stop, err := newOtelFileMeterProvider(cfg, testLogger())

		// ASSERT
		require.NoError(t, err)
		require.NotNil(t, meterProvider)
		require.NotNil(t, stop)
		require.NoError(t, stop())
	})

	t.Run("rejectsEmptyEnvironmentOverride", func(t *testing.T) {
		// ARRANGE
		t.Setenv("OTEL_CONFIG_FILE", "")
		path := writeOtelConfig(t, "file_format: \"1.0-rc.2\"\nmeter_provider: {}\n")
		cfg := config.Observability{Type: config.ObservabilityOTEL, OtelFile: path}

		// ACT
		meterProvider, stop, err := newOtelFileMeterProvider(cfg, testLogger())

		// ASSERT
		require.ErrorContains(t, err, "empty env OTEL_CONFIG_FILE=''")
		assert.Nil(t, meterProvider)
		assert.Nil(t, stop)
	})

	t.Run("acceptsLocalStackConfig", func(t *testing.T) {
		// ARRANGE
		bz, err := os.ReadFile("../../scripts/otel/ibc-otel.yaml")
		require.NoError(t, err)

		// ACT
		_, err = otelconf.ParseYAML(bz)

		// ASSERT
		require.NoError(t, err)
	})
}

func TestSimpleMeterProviderStop(t *testing.T) {
	// ARRANGE
	cfg := config.Observability{
		Type:                    config.ObservabilitySimple,
		SimpleMetricsListenAddr: availableListenAddress(t),
	}
	_, stop, err := newSimpleMeterProvider(cfg, testLogger())
	require.NoError(t, err)

	// ACT & ASSERT
	require.NoError(t, stop())
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

func writeOtelConfig(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "otel.yml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}
