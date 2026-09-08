// SPDX-License-Identifier: Apache-2.0

package otel

import (
	"io"
	"log/slog"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/internal/config"
)

func TestProvider(t *testing.T) {
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

func TestSimpleMeterProviderStop(t *testing.T) {
	// ARRANGE
	cfg := config.Observability{
		Type:          config.ObservabilitySimple,
		ListenAddress: availableListenAddress(t),
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
