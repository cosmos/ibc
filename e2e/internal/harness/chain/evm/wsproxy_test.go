// SPDX-License-Identifier: Apache-2.0

package evm_test

import (
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	chainevm "github.com/cosmos/ibc/e2e/internal/harness/chain/evm"
)

func TestWebSocketProxy(t *testing.T) {
	t.Parallel()

	t.Run("forwardBidirectional", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		ts := newWSProxyTestSuite(t)

		// ACT
		conn := ts.dial()
		defer conn.Close() //nolint:errcheck
		ts.roundTrip(conn, "ping")

		// ASSERT
		assert.Equal(t, "ws://"+ts.proxyAddr, ts.proxy.URL())
	})

	t.Run("killClosesActiveConnections", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		ts := newWSProxyTestSuite(t)
		conn := ts.dial()

		// ACT
		ts.proxy.Kill()

		// ASSERT
		require.Eventually(t, func() bool {
			_, readErr := conn.Read(make([]byte, 1))
			return readErr != nil
		}, time.Second, 10*time.Millisecond)
	})

	t.Run("killRejectsNewConnections", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		ts := newWSProxyTestSuite(t)
		ts.proxy.Kill()

		// ACT
		conn := ts.dial()

		// ASSERT
		require.Eventually(t, func() bool {
			_, readErr := conn.Read(make([]byte, 1))
			return readErr != nil
		}, time.Second, 10*time.Millisecond)
	})

	t.Run("reviveRestoresForwarding", func(t *testing.T) {
		t.Parallel()

		// ARRANGE #1
		ts := newWSProxyTestSuite(t)
		conn := ts.dial()
		ts.proxy.Kill()
		require.Eventually(t, func() bool {
			_, readErr := conn.Read(make([]byte, 1))
			return readErr != nil
		}, time.Second, 10*time.Millisecond)

		// ACT
		ts.proxy.Revive()

		// ASSERT
		conn = ts.dial()
		defer conn.Close() //nolint:errcheck
		ts.roundTrip(conn, "after-revive")
	})

	t.Run("killAndReviveAreIdempotent", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		ts := newWSProxyTestSuite(t)
		conn := ts.dial()

		// ACT #1
		ts.proxy.Kill()
		ts.proxy.Kill()

		// ASSERT #1
		require.Eventually(t, func() bool {
			_, readErr := conn.Read(make([]byte, 1))
			return readErr != nil
		}, time.Second, 10*time.Millisecond)

		// ACT #2
		ts.proxy.Revive()
		ts.proxy.Revive()

		// ASSERT #2
		conn = ts.dial()
		defer conn.Close() //nolint:errcheck
		ts.roundTrip(conn, "idempotent")
	})

	for _, tt := range []struct {
		name   string
		source string
	}{
		{
			name:   "wsScheme",
			source: "ws://",
		},
		{
			name:   "wssScheme",
			source: "wss://",
		},
	} {
		t.Run("acceptsSourceURL/"+tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			upstream, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			t.Cleanup(func() { _ = upstream.Close() })
			go serveEcho(upstream)

			proxy := chainevm.NewWebSocketProxy(t, tt.source+upstream.Addr().String())
			proxyAddr := strings.TrimPrefix(proxy.URL(), "ws://")

			// ACT
			conn, dialErr := net.Dial("tcp", proxyAddr)
			require.NoError(t, dialErr)
			defer conn.Close() //nolint:errcheck
			_, writeErr := conn.Write([]byte("ok"))
			require.NoError(t, writeErr)

			// ASSERT
			buf := make([]byte, 2)
			_, readErr := io.ReadFull(conn, buf)
			require.NoError(t, readErr)
			assert.Equal(t, "ok", string(buf))
		})
	}
}

type wsProxyTestSuite struct {
	t         *testing.T
	upstream  net.Listener
	proxy     *chainevm.WebSocketProxy
	proxyAddr string
}

func newWSProxyTestSuite(t *testing.T) *wsProxyTestSuite {
	t.Helper()

	upstream, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = upstream.Close() })

	ts := &wsProxyTestSuite{
		t:        t,
		upstream: upstream,
	}
	go serveEcho(upstream)

	ts.proxy = chainevm.NewWebSocketProxy(t, "ws://"+upstream.Addr().String())
	ts.proxyAddr = strings.TrimPrefix(ts.proxy.URL(), "ws://")

	return ts
}

func serveEcho(listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) {
			defer c.Close() //nolint:errcheck
			_, _ = io.Copy(c, c)
		}(conn)
	}
}

func (ts *wsProxyTestSuite) dial() net.Conn {
	ts.t.Helper()

	conn, err := net.Dial("tcp", ts.proxyAddr)
	require.NoError(ts.t, err)

	return conn
}

func (ts *wsProxyTestSuite) roundTrip(conn net.Conn, payload string) {
	ts.t.Helper()

	_, err := conn.Write([]byte(payload))
	require.NoError(ts.t, err)

	buf := make([]byte, len(payload))
	_, err = io.ReadFull(conn, buf)
	require.NoError(ts.t, err)
	assert.Equal(ts.t, payload, string(buf))
}
