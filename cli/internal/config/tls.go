// SPDX-License-Identifier: Apache-2.0

package config

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/cosmos/ibc/cli/internal/network"
)

// TLSClientConfig holds the settings for an outbound TLS connection. For every
// remote service an https:// address turns TLS on, see usesTLS; the block only
// customizes it and requires one. Without the block, or with an empty one, TLS
// uses system roots and no client certificate.
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
	_, err := c.TLSConfig()

	return err
}

// TLSConfig resolves the block into a *tls.Config, or nil when the block is
// absent, leaving the transport's defaults in place. The CA bundle and client
// certificate are read once here, so a rotated certificate takes effect only
// after a restart. A client certificate outside its validity period is
// rejected here rather than presented for the server to reject with a generic
// alert.
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
	case c.InsecureSkipVerify && c.CAFile != "":
		return nil, errPathf("caFile", "must not be set when insecureSkipVerify is enabled")
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

	// Leaf is left nil under GODEBUG=x509keypairleaf=0.
	leaf := cert.Leaf
	if leaf == nil {
		if leaf, err = x509.ParseCertificate(cert.Certificate[0]); err != nil {
			return nil, fmt.Errorf("parse client certificate: %w", err)
		}
	}

	now := time.Now()

	switch {
	case now.After(leaf.NotAfter):
		return nil, fmt.Errorf("client certificate %s expired %s", certFile, leaf.NotAfter.Format(time.RFC3339))
	case now.Before(leaf.NotBefore):
		return nil, fmt.Errorf(
			"client certificate %s is not valid until %s",
			certFile, leaf.NotBefore.Format(time.RFC3339),
		)
	}

	cfg.Certificates = []tls.Certificate{cert}

	return cfg, nil
}

// ResolveEndpoint pairs addr with its TLS settings: for an https:// addr,
// tlsCfg resolved by TLSConfig or Go's defaults when it is nil; for any other
// addr none. Warns through logger when server verification is disabled.
// logAttrs identify the endpoint in that warning; keep URLs out of them, since
// a URL can carry credentials in its userinfo.
func ResolveEndpoint(
	addr string,
	tlsCfg *TLSClientConfig,
	logger *slog.Logger,
	logAttrs ...any,
) (network.Endpoint, error) {
	if err := checkTLSAddr(addr, tlsCfg); err != nil {
		return network.Endpoint{}, err
	}

	if !usesTLS(addr) {
		return network.Endpoint{URL: addr}, nil
	}

	tlsConfig, err := tlsCfg.TLSConfig()
	if err != nil {
		return network.Endpoint{}, errPath("tls", err)
	}

	if tlsConfig == nil {
		tlsConfig = &tls.Config{}
	}

	if tlsConfig.InsecureSkipVerify {
		if logger == nil {
			logger = slog.Default()
		}

		logger.Warn("TLS server certificate verification is disabled", logAttrs...)
	}

	return network.Endpoint{URL: addr, TLS: tlsConfig}, nil
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

// usesTLS reports whether addr is an https:// URL, which is what turns TLS on
// for every remote service.
func usesTLS(addr string) bool {
	return strings.HasPrefix(strings.ToLower(addr), "https://")
}

// endpointURL returns addr as a URL: as written when it has a scheme, and as
// http:// for a bare host:port.
func endpointURL(addr string) string {
	if strings.Contains(addr, "://") {
		return addr
	}

	return "http://" + addr
}

// isHTTPURL reports whether addr is an http:// or https:// URL.
func isHTTPURL(addr string) bool {
	return usesTLS(addr) || strings.HasPrefix(strings.ToLower(addr), "http://")
}

// checkHTTPURL validates an http(s):// addr: it must parse and name a host,
// and carry no query or fragment, which would swallow the RPC path appended
// to it. targetOnly also rejects userinfo and a path, which a gRPC target
// can't carry. Any other addr is left alone. Errors don't echo addr, which can
// carry credentials.
func checkHTTPURL(addr string, targetOnly bool) error {
	if !isHTTPURL(addr) {
		return nil
	}

	u, err := url.Parse(addr)

	switch {
	case err != nil:
		return errors.New("invalid URL")
	case u.Hostname() == "":
		return errors.New("must include a host")
	case u.RawQuery != "" || u.ForceQuery || strings.Contains(addr, "#"):
		return errors.New("must not have a query or fragment")
	case targetOnly && u.User != nil:
		return errors.New("must not have userinfo")
	case targetOnly && strings.Trim(u.Path, "/") != "":
		return errors.New("must not have a path")
	}

	return nil
}

// checkTLSAddr rejects a tls block on an address that doesn't use TLS, where
// its settings would be silently ignored.
func checkTLSAddr(addr string, tlsCfg *TLSClientConfig) error {
	if tlsCfg != nil && !usesTLS(addr) {
		return errPathf("tls", "requires an https:// address")
	}

	return nil
}

// validateEndpointTLS checks tlsCfg against addr and loads its files.
func validateEndpointTLS(addr string, tlsCfg *TLSClientConfig) error {
	if err := checkTLSAddr(addr, tlsCfg); err != nil {
		return err
	}

	return errPath("tls", tlsCfg.Validate())
}
