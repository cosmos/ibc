// SPDX-License-Identifier: Apache-2.0

package network_test

import (
	"crypto/tls"
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
		require.Nil(t, cfg.GetClientCertificate)
		require.False(t, cfg.InsecureSkipVerify)
	})

	t.Run("carries every option through", func(t *testing.T) {
		t.Parallel()

		cfg, err := network.BuildClientTLS(network.ClientTLS{
			CAFile:             caFile,
			CertFile:           certFile,
			KeyFile:            keyFile,
			ServerName:         "attestor.example.com",
			MinVersion:         network.TLSVersion13,
			InsecureSkipVerify: true,
		})
		require.NoError(t, err)
		require.Equal(t, uint16(tls.VersionTLS13), cfg.MinVersion)
		require.Equal(t, "attestor.example.com", cfg.ServerName)
		require.True(t, cfg.InsecureSkipVerify)
		require.NotNil(t, cfg.RootCAs)
		require.NotNil(t, cfg.GetClientCertificate)
	})

	t.Run("rejects a bad version", func(t *testing.T) {
		t.Parallel()

		_, err := network.BuildClientTLS(network.ClientTLS{MinVersion: "1.1"})
		require.Error(t, err)
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

// The certificate is reloaded per handshake so rotation on disk takes effect
// without restarting the process.
func TestBuildClientTLSReloadsCertificate(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	certFile, keyFile := certs.WriteSelfSigned(t, dir, "first")

	cfg, err := network.BuildClientTLS(network.ClientTLS{CertFile: certFile, KeyFile: keyFile})
	require.NoError(t, err)

	first, err := cfg.GetClientCertificate(&tls.CertificateRequestInfo{})
	require.NoError(t, err)

	// Rotate the pair in place, as a cert manager would.
	rotatedCert, rotatedKey := certs.WriteSelfSigned(t, dir, "second")
	require.NoError(t, os.Rename(rotatedCert, certFile))
	require.NoError(t, os.Rename(rotatedKey, keyFile))

	second, err := cfg.GetClientCertificate(&tls.CertificateRequestInfo{})
	require.NoError(t, err)

	require.NotEqual(t, first.Certificate[0], second.Certificate[0], "expected the rotated certificate")
}
