// SPDX-License-Identifier: Apache-2.0

package network_test

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/internal/network"
	"github.com/cosmos/ibc/cli/internal/testutil/certs"
)

func TestParseTLSVersion(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		raw    string
		expect uint16
	}{
		{"", tls.VersionTLS12},
		{"1.2", tls.VersionTLS12},
		{"1.3", tls.VersionTLS13},
	} {
		got, err := network.ParseTLSVersion(tc.raw)
		require.NoError(t, err)
		require.Equal(t, tc.expect, got)
	}

	// HTTP/2 needs at least 1.2, and gRPC needs HTTP/2.
	for _, raw := range []string{"1.1", "1.0", "1.4", "TLS1.2", "nonsense"} {
		_, err := network.ParseTLSVersion(raw)
		require.Error(t, err, raw)
		require.Contains(t, err.Error(), "TLS 1.2")
	}
}

func TestBuildClientTLS(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	certFile, keyFile := certs.WriteSelfSigned(t, dir, "client")
	caFile := certFile

	t.Run("defaults to TLS 1.2 with system roots", func(t *testing.T) {
		t.Parallel()

		cfg, err := network.BuildClientTLS(network.ClientTLS{})
		require.NoError(t, err)
		require.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)
		require.Nil(t, cfg.RootCAs)
		require.Empty(t, cfg.Certificates)
		require.False(t, cfg.InsecureSkipVerify)
	})

	t.Run("carries every option through", func(t *testing.T) {
		t.Parallel()

		cfg, err := network.BuildClientTLS(network.ClientTLS{
			CAFile:             caFile,
			CertFile:           certFile,
			KeyFile:            keyFile,
			ServerName:         "attestor.example.com",
			MinVersion:         tls.VersionTLS13,
			InsecureSkipVerify: true,
		})
		require.NoError(t, err)
		require.Equal(t, uint16(tls.VersionTLS13), cfg.MinVersion)
		require.Equal(t, "attestor.example.com", cfg.ServerName)
		require.True(t, cfg.InsecureSkipVerify)
		require.NotNil(t, cfg.RootCAs)
		require.Len(t, cfg.Certificates, 1)
	})

	t.Run("rejects a missing CA file", func(t *testing.T) {
		t.Parallel()

		_, err := network.BuildClientTLS(network.ClientTLS{CAFile: filepath.Join(dir, "absent.crt")})
		require.ErrorContains(t, err, "read CA file")
	})

	t.Run("rejects a CA file holding no PEM", func(t *testing.T) {
		t.Parallel()

		junk := filepath.Join(dir, "junk.crt")
		require.NoError(t, os.WriteFile(junk, []byte("not a certificate"), 0o600))

		_, err := network.BuildClientTLS(network.ClientTLS{CAFile: junk})
		require.ErrorContains(t, err, "no PEM certificates")
	})

	t.Run("rejects an unloadable key pair up front", func(t *testing.T) {
		t.Parallel()

		_, err := network.BuildClientTLS(network.ClientTLS{
			CertFile: certFile,
			KeyFile:  filepath.Join(dir, "absent.key"),
		})
		require.ErrorContains(t, err, "load client certificate")
	})
}

// A pair that can never be presented successfully fails here, naming the
// file, since it is not reloaded later.
func TestBuildClientTLSRejectsExpiredCertificate(t *testing.T) {
	t.Parallel()

	certFile, keyFile := certs.WriteExpiredSelfSigned(t, t.TempDir(), "expired")

	_, err := network.BuildClientTLS(network.ClientTLS{CertFile: certFile, KeyFile: keyFile})
	require.ErrorContains(t, err, "client certificate "+certFile+" expired")
}

func TestBuildClientTLSRejectsKeyWithoutCert(t *testing.T) {
	t.Parallel()

	_, keyFile := certs.WriteSelfSigned(t, t.TempDir(), "client")

	_, err := network.BuildClientTLS(network.ClientTLS{KeyFile: keyFile})
	require.ErrorContains(t, err, "client certificate and key must be set together")
}

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
