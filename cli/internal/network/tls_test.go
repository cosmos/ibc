// SPDX-License-Identifier: Apache-2.0

package network_test

import (
	"crypto/tls"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

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
			MinVersion:         tls.VersionTLS13,
			InsecureSkipVerify: true,
		})
		require.NoError(t, err)
		require.Equal(t, uint16(tls.VersionTLS13), cfg.MinVersion)
		require.Equal(t, "attestor.example.com", cfg.ServerName)
		require.True(t, cfg.InsecureSkipVerify)
		require.NotNil(t, cfg.RootCAs)
		require.NotNil(t, cfg.GetClientCertificate)
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

// The certificate is reloaded per handshake so a short-lived certificate
// rotated on disk takes effect without restarting the process.
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

	// A half-done rotation (new cert, old key) falls back to the last good pair.
	thirdCert, _ := certs.WriteSelfSigned(t, dir, "third")
	require.NoError(t, os.Rename(thirdCert, certFile))

	fallback, err := cfg.GetClientCertificate(&tls.CertificateRequestInfo{})
	require.NoError(t, err)
	require.Equal(t, second.Certificate[0], fallback.Certificate[0], "expected the last good certificate")
}

// Overlapping handshakes can finish their reloads out of order, so a reload
// that started first must not replace the pair a later one stored. The cert
// file is a FIFO here so the first reload can be held mid-read.
func TestBuildClientTLSFallbackIgnoresStaleReload(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	certFile, keyFile := certs.WriteSelfSigned(t, dir, "old")

	oldCertPEM, err := os.ReadFile(certFile)
	require.NoError(t, err)
	oldKeyPEM, err := os.ReadFile(keyFile)
	require.NoError(t, err)

	cfg, err := network.BuildClientTLS(network.ClientTLS{CertFile: certFile, KeyFile: keyFile})
	require.NoError(t, err)

	old, err := cfg.GetClientCertificate(&tls.CertificateRequestInfo{})
	require.NoError(t, err)

	fifo := filepath.Join(dir, "fifo")
	require.NoError(t, syscall.Mkfifo(fifo, 0o600))
	require.NoError(t, os.Rename(fifo, certFile))

	stale := make(chan *tls.Certificate, 1)
	go func() {
		cert, reloadErr := cfg.GetClientCertificate(&tls.CertificateRequestInfo{})
		if reloadErr != nil {
			t.Errorf("stale reload: %v", reloadErr)
		}
		stale <- cert
	}()

	// Opening the write end blocks until the stale reload opens the read end.
	w, err := os.OpenFile(certFile, os.O_WRONLY, 0)
	require.NoError(t, err)

	newCert, newKey := certs.WriteSelfSigned(t, dir, "new")
	require.NoError(t, os.Rename(newCert, certFile))
	require.NoError(t, os.Rename(newKey, keyFile))

	fresh, err := cfg.GetClientCertificate(&tls.CertificateRequestInfo{})
	require.NoError(t, err)

	// Let the stale reload finish, after the fresh one, with the old pair.
	require.NoError(t, os.WriteFile(keyFile, oldKeyPEM, 0o600))
	_, err = w.Write(oldCertPEM)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	require.Equal(t, old.Certificate[0], (<-stale).Certificate[0], "expected the stale reload to load the old pair")

	require.NoError(t, os.Remove(keyFile))

	fallback, err := cfg.GetClientCertificate(&tls.CertificateRequestInfo{})
	require.NoError(t, err)
	require.Equal(t, fresh.Certificate[0], fallback.Certificate[0], "expected the fresh reload's certificate")
}

// A rollback or reissue can carry an earlier issue date than the pair it
// replaces; once it loads, it is the fallback.
func TestBuildClientTLSFallbackFollowsRollback(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	certFile, keyFile := certs.WriteSelfSigned(t, dir, "newer")

	cfg, err := network.BuildClientTLS(network.ClientTLS{CertFile: certFile, KeyFile: keyFile})
	require.NoError(t, err)

	newer, err := cfg.GetClientCertificate(&tls.CertificateRequestInfo{})
	require.NoError(t, err)

	olderCert, olderKey := certs.WriteSelfSignedIssuedAt(t, dir, "older", time.Now().Add(-2*time.Hour))
	require.NoError(t, os.Rename(olderCert, certFile))
	require.NoError(t, os.Rename(olderKey, keyFile))

	rolledBack, err := cfg.GetClientCertificate(&tls.CertificateRequestInfo{})
	require.NoError(t, err)
	require.NotEqual(t, newer.Certificate[0], rolledBack.Certificate[0], "expected the rolled-back certificate")

	require.NoError(t, os.Remove(keyFile))

	fallback, err := cfg.GetClientCertificate(&tls.CertificateRequestInfo{})
	require.NoError(t, err)
	require.Equal(t, rolledBack.Certificate[0], fallback.Certificate[0], "expected the rolled-back certificate")
}

// A rotation that stops renewing leaves an intact but expired pair on disk;
// it loads, but is never presented.
func TestBuildClientTLSRejectsExpiredReload(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	certFile, keyFile := certs.WriteExpiredSelfSigned(t, dir, "expired")

	cfg, err := network.BuildClientTLS(network.ClientTLS{CertFile: certFile, KeyFile: keyFile})
	require.NoError(t, err)

	_, err = cfg.GetClientCertificate(&tls.CertificateRequestInfo{})
	require.ErrorContains(t, err, "client certificate "+certFile+" expired")
}

// A broken rotation falls back to the last good certificate only while it is
// still valid; after that the handshake fails with the reload error rather
// than presenting a certificate the server will reject for a vaguer reason.
func TestBuildClientTLSFallbackStopsAtExpiry(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	certFile, keyFile := certs.WriteExpiredSelfSigned(t, dir, "expired")

	cfg, err := network.BuildClientTLS(network.ClientTLS{CertFile: certFile, KeyFile: keyFile})
	require.NoError(t, err)

	require.NoError(t, os.Remove(keyFile))

	_, err = cfg.GetClientCertificate(&tls.CertificateRequestInfo{})
	require.ErrorContains(t, err, "last loaded one expired")
	require.ErrorContains(t, err, keyFile)
}

// Handshakes reload concurrently with a rotation in progress; run with -race.
func TestBuildClientTLSConcurrentReload(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	certFile, keyFile := certs.WriteSelfSigned(t, dir, "client")

	cfg, err := network.BuildClientTLS(network.ClientTLS{CertFile: certFile, KeyFile: keyFile})
	require.NoError(t, err)

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 50 {
				cert, err := cfg.GetClientCertificate(&tls.CertificateRequestInfo{})
				if err != nil || cert == nil || len(cert.Certificate) == 0 {
					t.Errorf("GetClientCertificate = %v, %v", cert, err)
					return
				}
			}
		})
	}

	for i := range 20 {
		rotatedCert, rotatedKey := certs.WriteSelfSigned(t, dir, fmt.Sprintf("rotated-%d", i))
		require.NoError(t, os.Rename(rotatedCert, certFile))
		require.NoError(t, os.Rename(rotatedKey, keyFile))
	}

	wg.Wait()
}
