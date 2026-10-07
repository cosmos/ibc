// SPDX-License-Identifier: Apache-2.0

package network

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"
)

// TLS version names accepted in configuration. Anything lower cannot carry
// HTTP/2, which gRPC requires.
const (
	TLSVersion12 = "1.2"
	TLSVersion13 = "1.3"
)

// ErrCAFile marks a BuildClientTLS failure caused by the CA file, so a caller
// can attribute it to its own config field.
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

// BuildClientTLS resolves opts into a *tls.Config. The CA bundle and client
// certificate are read once here, so a rotated certificate takes effect only
// after a restart. An expired client certificate is rejected here rather than
// presented for the server to reject with a generic alert.
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
		pool, poolErr := caPool(opts.CAFile)
		if poolErr != nil {
			return nil, poolErr
		}

		cfg.RootCAs = pool
	}

	switch {
	case opts.CertFile == "" && opts.KeyFile == "":
		return cfg, nil
	case opts.CertFile == "" || opts.KeyFile == "":
		return nil, errors.New("client certificate and key must be set together")
	}

	cert, err := tls.LoadX509KeyPair(opts.CertFile, opts.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load client certificate: %w", err)
	}

	if cert.Leaf != nil && time.Now().After(cert.Leaf.NotAfter) {
		return nil, fmt.Errorf(
			"client certificate %s expired %s",
			opts.CertFile, cert.Leaf.NotAfter.Format(time.RFC3339),
		)
	}

	cfg.Certificates = []tls.Certificate{cert}

	return cfg, nil
}

// NewGRPCHTTPClient returns a client for connect's gRPC protocol to endpoint.
// gRPC needs HTTP/2. Over TLS it is negotiated, with HTTP/1.1 also offered for
// servers that serve gRPC over it, such as connect-go. Plaintext has no
// negotiation and Go would always pick HTTP/1.1 there, so it is h2c only.
func NewGRPCHTTPClient(endpoint Endpoint) *http.Client {
	protocols := new(http.Protocols)

	if u, err := url.Parse(endpoint.URL); err == nil && u.Scheme == "https" {
		protocols.SetHTTP1(true)
		protocols.SetHTTP2(true)
	} else {
		protocols.SetUnencryptedHTTP2(true)
	}

	return &http.Client{Transport: &http.Transport{Protocols: protocols, TLSClientConfig: endpoint.TLS}}
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

// caPool loads caFile into a cert pool verifying a TLS server.
func caPool(caFile string) (*x509.CertPool, error) {
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
