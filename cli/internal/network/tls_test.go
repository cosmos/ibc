// SPDX-License-Identifier: Apache-2.0

package network_test

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/internal/network"
)

// Over TLS the server picks HTTP/2 or HTTP/1.1; plaintext is h2c only.
func TestNewGRPCHTTPClientProtocols(t *testing.T) {
	t.Parallel()

	proto := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, r.Proto)
	})

	get := func(t *testing.T, endpoint network.Endpoint) string {
		t.Helper()

		resp, err := network.NewGRPCHTTPClient(endpoint).Get(endpoint.URL)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		return string(body)
	}

	startTLS := func(t *testing.T, http2 bool) network.Endpoint {
		t.Helper()

		srv := httptest.NewUnstartedServer(proto)
		srv.EnableHTTP2 = http2
		srv.StartTLS()
		t.Cleanup(srv.Close)

		roots := x509.NewCertPool()
		roots.AddCert(srv.Certificate())

		return network.Endpoint{URL: srv.URL, TLS: &tls.Config{RootCAs: roots}}
	}

	t.Run("TLS server with HTTP/2", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "HTTP/2.0", get(t, startTLS(t, true)))
	})

	t.Run("TLS server with only HTTP/1.1", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "HTTP/1.1", get(t, startTLS(t, false)))
	})

	t.Run("plaintext server with HTTP/1.1 and h2c", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewUnstartedServer(proto)
		srv.Config.Protocols = new(http.Protocols)
		srv.Config.Protocols.SetHTTP1(true)
		srv.Config.Protocols.SetUnencryptedHTTP2(true)
		srv.Start()
		t.Cleanup(srv.Close)

		require.Equal(t, "HTTP/2.0", get(t, network.Endpoint{URL: srv.URL}))
	})
}
