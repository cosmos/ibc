// SPDX-License-Identifier: Apache-2.0

package config

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/cosmos/ibc/cli/internal/network"
)

// TLSClientConfig holds the settings for an outbound TLS connection.
//
// The block carries settings. Whether TLS is used depends on the endpoint:
//   - a remote prover url with a scheme: the scheme. https:// uses TLS with
//     Go's defaults even without the block; the block with http:// is rejected.
//   - a remote attestor grpc address, or a remote prover url written as a bare
//     host:port: the block's presence, which selects https over http.
//   - a remote signer grpc target: the block's presence, which selects TLS
//     credentials over plaintext, whatever scheme the target has.
//
// An empty block (`tls: {}`) is valid and means system roots and no client
// certificate.
type TLSClientConfig struct {
	// CAFile is a PEM bundle verifying the server. Empty uses system roots.
	// Read at startup, so a changed bundle needs a restart.
	CAFile string `yaml:"caFile,omitempty"`

	// CertFile is the client certificate presented for mTLS. Required with
	// keyFile. Omit both for one-way TLS. Read at startup, so a rotated
	// certificate needs a restart.
	CertFile string `yaml:"certFile,omitempty"`

	// KeyFile is the private key for certFile. Required with certFile.
	KeyFile string `yaml:"keyFile,omitempty"`

	// ServerName overrides the name verified against the server certificate.
	// Needed when dialing an address that differs from the certificate's name.
	ServerName string `yaml:"serverName,omitempty"`

	// InsecureSkipVerify disables server certificate verification and logs a
	// warning. Development only. Must not be combined with caFile.
	InsecureSkipVerify bool `yaml:"insecureSkipVerify,omitempty"`
}

// Validate reports whether the block is internally consistent and its files
// are readable and well-formed, so a bad mount or a mismatched cert/key pair
// fails at config load rather than at first use. A nil block is valid. It
// does not warn about InsecureSkipVerify; the caller that goes on to actually
// build a connection with the result does that.
func (c *TLSClientConfig) Validate() error {
	if c == nil {
		return nil
	}

	if c.InsecureSkipVerify && c.CAFile != "" {
		return errPathf("caFile", "must not be set when insecureSkipVerify is enabled")
	}

	_, err := c.TLSConfig()

	return err
}

// TLSConfig resolves the block into a *tls.Config, or nil when the block is
// absent, leaving the transport's defaults in place. The CA bundle and client
// certificate are read once here, so a rotated certificate takes effect only
// after a restart. An expired client certificate is rejected here rather than
// presented for the server to reject with a generic alert.
//
// A client certificate error isn't attributed to a field: a mismatched pair
// or malformed PEM could be either file's fault, and the error already names
// the file it failed to read.
func (c *TLSClientConfig) TLSConfig() (*tls.Config, error) {
	if c == nil {
		return nil, nil
	}

	// Checked here rather than in Validate, which a load can skip, so a
	// half-configured client certificate never yields one-way TLS.
	switch {
	case c.CertFile != "" && c.KeyFile == "":
		return nil, errPathf("keyFile", "required when certFile is set")
	case c.KeyFile != "" && c.CertFile == "":
		return nil, errPathf("certFile", "required when keyFile is set")
	}

	cfg := &tls.Config{
		ServerName:         c.ServerName,
		InsecureSkipVerify: c.InsecureSkipVerify,
	}

	if c.CAFile != "" {
		pool, err := caPool(c.CAFile)
		if err != nil {
			return nil, errPath("caFile", err)
		}

		cfg.RootCAs = pool
	}

	if c.CertFile == "" {
		return cfg, nil
	}

	certFile, err := ExpandHome(c.CertFile)
	if err != nil {
		return nil, errPath("certFile", err)
	}

	keyFile, err := ExpandHome(c.KeyFile)
	if err != nil {
		return nil, errPath("keyFile", err)
	}

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load client certificate: %w", err)
	}

	if cert.Leaf != nil && time.Now().After(cert.Leaf.NotAfter) {
		return nil, fmt.Errorf(
			"client certificate %s expired %s",
			certFile, cert.Leaf.NotAfter.Format(time.RFC3339),
		)
	}

	cfg.Certificates = []tls.Certificate{cert}

	return cfg, nil
}

// caPool loads caFile into a cert pool verifying a TLS server.
func caPool(caFile string) (*x509.CertPool, error) {
	path, err := ExpandHome(caFile)
	if err != nil {
		return nil, err
	}

	bz, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read CA file: %w", err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(bz) {
		return nil, fmt.Errorf("CA file %q contains no PEM certificates", path)
	}

	return pool, nil
}

// ResolveEndpoint pairs url with the resolved form of tlsCfg, see TLSConfig,
// and warns through logger when server verification is disabled.
// logAttrs identify the endpoint in that warning; keep URLs out of them, since
// a URL can carry credentials in its userinfo.
func ResolveEndpoint(
	url string,
	tlsCfg *TLSClientConfig,
	logger *slog.Logger,
	logAttrs ...any,
) (network.Endpoint, error) {
	tlsConfig, err := tlsCfg.TLSConfig()
	if err != nil {
		return network.Endpoint{}, errPath("tls", err)
	}

	if tlsConfig != nil && tlsConfig.InsecureSkipVerify {
		if logger == nil {
			logger = slog.Default()
		}

		logger.Warn("TLS server certificate verification is disabled", logAttrs...)
	}

	return network.Endpoint{URL: url, TLS: tlsConfig}, nil
}
