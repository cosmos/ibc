// SPDX-License-Identifier: Apache-2.0

package config

import (
	"crypto/tls"
	"errors"
	"log/slog"

	"github.com/cosmos/ibc/cli/internal/network"
)

// TLSClientConfig configures an outbound TLS connection.
//
// Presence of the block is what enables TLS; there is no separate flag. For an
// endpoint configured as a bare host:port it selects https over http. For one
// configured as a URL the scheme already decides, and the block only supplies
// the CA and client certificate.
//
// An empty block (`tls: {}`) is valid and means TLS with system roots, no
// client certificate, and the default version floor.
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

	// MinVersion is "1.2" (default) or "1.3". Lower is rejected: gRPC needs
	// HTTP/2, and HTTP/2 needs at least TLS 1.2.
	MinVersion string `yaml:"minVersion,omitempty"`

	// InsecureSkipVerify disables server certificate verification and logs a
	// warning. Development only. Must not be combined with caFile.
	InsecureSkipVerify bool `yaml:"insecureSkipVerify,omitempty"`
}

// Validate reports whether the block is internally consistent and its files
// are readable and well-formed, so a bad mount or a mismatched cert/key pair
// fails at config load rather than at first use. A nil block is valid and
// means plaintext. It does not warn about InsecureSkipVerify; the caller
// that goes on to actually build a connection with the result does that.
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
// absent, meaning the connection stays plaintext. A CA file error is reported
// against caFile. A client certificate error isn't attributed to a field: a
// mismatched pair or malformed PEM could be either file's fault, and the
// error already names the file it failed to read.
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

	minVersion, err := network.ParseTLSVersion(c.MinVersion)
	if err != nil {
		return nil, errPath("minVersion", err)
	}

	caFile, err := ExpandHome(c.CAFile)
	if err != nil {
		return nil, errPath("caFile", err)
	}

	certFile, err := ExpandHome(c.CertFile)
	if err != nil {
		return nil, errPath("certFile", err)
	}

	keyFile, err := ExpandHome(c.KeyFile)
	if err != nil {
		return nil, errPath("keyFile", err)
	}

	cfg, err := network.BuildClientTLS(network.ClientTLS{
		CAFile:             caFile,
		CertFile:           certFile,
		KeyFile:            keyFile,
		ServerName:         c.ServerName,
		MinVersion:         minVersion,
		InsecureSkipVerify: c.InsecureSkipVerify,
	})
	if errors.Is(err, network.ErrCAFile) {
		return nil, errPath("caFile", err)
	}

	return cfg, err
}

// ResolveEndpoint pairs url with the resolved form of tlsCfg (nil meaning
// plaintext) and warns through logger when server verification is disabled.
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
