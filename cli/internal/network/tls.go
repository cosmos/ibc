// SPDX-License-Identifier: Apache-2.0

package network

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"time"
)

// TLS version names accepted in configuration. Anything lower cannot carry
// HTTP/2, which gRPC requires.
const (
	TLSVersion12 = "1.2"
	TLSVersion13 = "1.3"
)

// ErrCAFile marks a BuildClientTLS or CAPool failure caused by the CA file, so
// a caller can attribute it to its own config field.
var ErrCAFile = errors.New("CA file")

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

	// MinVersion is a tls.VersionTLS* constant, as ParseTLSVersion returns.
	// Zero defaults to TLS 1.2.
	MinVersion uint16

	// InsecureSkipVerify disables server certificate verification.
	InsecureSkipVerify bool
}

// Endpoint is a remote service address paired with its resolved TLS
// settings. A nil TLS means plaintext.
type Endpoint struct {
	URL string
	TLS *tls.Config
}

// BuildClientTLS resolves opts into a *tls.Config. A client certificate is
// loaded once here so a bad pair fails immediately, then reloaded per
// handshake so short-lived certificates rotated on disk are presented on the
// next connection without restarting the process. CA roots require a
// restart. A reload that fails (for example, because it raced a separate
// cert and key update) falls back to the certificate from the latest-started
// reload that succeeded, until that one expires; from then on the handshake
// fails with the reload error, so a rotation that stays broken surfaces its
// cause. A reload that reads an expired certificate fails the handshake
// naming it, rather than presenting it for the server to reject with a
// generic alert. Loading one here is not an error, so a process started
// before renewal picks up the renewed pair on its next connection.
//
// This does not warn about InsecureSkipVerify: it runs both at config
// validation and at connect time, so the caller that knows it's about to
// actually use the result is responsible for that warning.
func BuildClientTLS(opts ClientTLS) (*tls.Config, error) {
	minVersion := opts.MinVersion
	if minVersion == 0 {
		minVersion = tls.VersionTLS12
	}

	cfg := &tls.Config{
		MinVersion:         minVersion,
		ServerName:         opts.ServerName,
		InsecureSkipVerify: opts.InsecureSkipVerify,
	}

	if opts.CAFile != "" {
		pool, poolErr := CAPool(opts.CAFile)
		if poolErr != nil {
			return nil, poolErr
		}

		cfg.RootCAs = pool
	}

	if opts.CertFile == "" {
		return cfg, nil
	}

	certFile, keyFile := opts.CertFile, opts.KeyFile

	first, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load client certificate: %w", err)
	}

	// loaded is a pair and the order its reload started in. None is modified
	// after being stored, so concurrent handshakes can share whichever one
	// they load.
	type loaded struct {
		cert *tls.Certificate
		seq  uint64
	}

	var (
		reloads atomic.Uint64
		last    atomic.Pointer[loaded]
	)
	last.Store(&loaded{cert: &first})

	cfg.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
		seq := reloads.Add(1)

		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err == nil && cert.Leaf != nil && time.Now().After(cert.Leaf.NotAfter) {
			return nil, fmt.Errorf(
				"client certificate %s expired %s",
				certFile, cert.Leaf.NotAfter.Format(time.RFC3339),
			)
		}

		if err == nil {
			// Overlapping reloads can finish out of order, so one doesn't
			// replace a pair stored by a reload that started after it.
			next := &loaded{cert: &cert, seq: seq}
			for cur := last.Load(); cur.seq < seq; cur = last.Load() {
				if last.CompareAndSwap(cur, next) {
					break
				}
			}

			return &cert, nil
		}

		fallback := last.Load().cert
		if fallback.Leaf != nil && time.Now().After(fallback.Leaf.NotAfter) {
			return nil, fmt.Errorf(
				"reload client certificate (last loaded one expired %s): %w",
				fallback.Leaf.NotAfter.Format(time.RFC3339), err,
			)
		}

		slog.Warn(
			"Reloading client certificate failed, using last loaded certificate",
			"certFile", certFile, "keyFile", keyFile, "err", err,
		)

		return fallback, nil
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

// CAPool loads caFile into a cert pool verifying a TLS server, exported so
// config validation can surface a parse failure against the caFile field
// without duplicating this logic.
func CAPool(caFile string) (*x509.CertPool, error) {
	bz, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("read %w: %w", ErrCAFile, err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(bz) {
		return nil, fmt.Errorf("%w %q contains no PEM certificates", ErrCAFile, caFile)
	}

	return pool, nil
}
