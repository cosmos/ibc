// SPDX-License-Identifier: Apache-2.0

package network

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"os"
)

// TLS version names accepted in configuration. Anything lower cannot carry
// HTTP/2, which gRPC requires.
const (
	TLSVersion12 = "1.2"
	TLSVersion13 = "1.3"
)

// ClientTLS describes an outbound TLS connection, decoupled from config file
// shape so this package stays free of a config dependency.
type ClientTLS struct {
	// CAFile is a PEM bundle verifying the server. Empty uses system roots.
	CAFile string

	// CertFile and KeyFile are the client certificate presented for mTLS.
	CertFile string
	KeyFile  string

	// ServerName overrides the name verified against the server certificate.
	ServerName string

	// MinVersion is a TLSVersion* name. Empty defaults to TLS 1.2.
	MinVersion string

	// InsecureSkipVerify disables server certificate verification.
	InsecureSkipVerify bool
}

// BuildClientTLS resolves opts into a *tls.Config. A client certificate is
// loaded once here so a bad pair fails immediately, then reloaded per
// handshake so rotation on disk takes effect without restarting the process.
// CA roots require a restart. Updating cert and key separately can briefly
// cause handshakes to fail if they observe a mismatched pair.
func BuildClientTLS(opts ClientTLS) (*tls.Config, error) {
	if opts.InsecureSkipVerify {
		slog.Warn("TLS server certificate verification is disabled", "serverName", opts.ServerName)
	}

	minVersion, err := ParseTLSVersion(opts.MinVersion)
	if err != nil {
		return nil, err
	}

	cfg := &tls.Config{
		MinVersion:         minVersion,
		ServerName:         opts.ServerName,
		InsecureSkipVerify: opts.InsecureSkipVerify,
	}

	if opts.CAFile != "" {
		pool, err := certPool(opts.CAFile)
		if err != nil {
			return nil, err
		}

		cfg.RootCAs = pool
	}

	if opts.CertFile == "" {
		return cfg, nil
	}

	certFile, keyFile := opts.CertFile, opts.KeyFile
	if _, err := tls.LoadX509KeyPair(certFile, keyFile); err != nil {
		return nil, fmt.Errorf("load client certificate: %w", err)
	}

	cfg.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, fmt.Errorf("reload client certificate: %w", err)
		}

		return &cert, nil
	}

	return cfg, nil
}

// ParseTLSVersion maps a configured version name to its tls constant. Empty
// defaults to TLS 1.2, matching Go's own default floor.
func ParseTLSVersion(raw string) (uint16, error) {
	switch raw {
	case "", TLSVersion12:
		return tls.VersionTLS12, nil
	case TLSVersion13:
		return tls.VersionTLS13, nil
	default:
		return 0, fmt.Errorf(
			"must be one of [%q, %q], got %q: HTTP/2 requires at least TLS 1.2 and gRPC requires HTTP/2",
			TLSVersion12, TLSVersion13, raw,
		)
	}
}

func certPool(caFile string) (*x509.CertPool, error) {
	bz, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("read CA file: %w", err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(bz) {
		return nil, fmt.Errorf("CA file %q contains no PEM certificates", caFile)
	}

	return pool, nil
}
