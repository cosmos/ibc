// SPDX-License-Identifier: Apache-2.0

package config

import (
	"crypto/tls"
	"os"

	"github.com/cosmos/ibc/cli/internal/network"
)

// yaml field names, used to attach a config path to a validation error.
const (
	fieldCAFile   = "caFile"
	fieldCertFile = "certFile"
	fieldKeyFile  = "keyFile"
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
	CAFile string `yaml:"caFile,omitempty"`

	// CertFile and KeyFile are the client certificate presented for mTLS.
	// Required together. Omit both for one-way TLS.
	CertFile string `yaml:"certFile,omitempty"`
	KeyFile  string `yaml:"keyFile,omitempty"`

	// ServerName overrides the name verified against the server certificate.
	// Needed when dialing an address that differs from the certificate's name.
	ServerName string `yaml:"serverName,omitempty"`

	// MinVersion is "1.2" (default) or "1.3". Lower is rejected: gRPC needs
	// HTTP/2, and HTTP/2 needs at least TLS 1.2.
	MinVersion string `yaml:"minVersion,omitempty"`

	// InsecureSkipVerify disables server certificate verification. Development only.
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

	switch {
	case c.InsecureSkipVerify && c.CAFile != "":
		return errPathf(fieldCAFile, "must not be set when insecureSkipVerify is enabled")
	case c.CertFile != "" && c.KeyFile == "":
		return errPathf(fieldKeyFile, "required when certFile is set")
	case c.KeyFile != "" && c.CertFile == "":
		return errPathf(fieldCertFile, "required when keyFile is set")
	}

	if _, err := network.ParseTLSVersion(c.MinVersion); err != nil {
		return errPath("minVersion", err)
	}

	if c.CAFile != "" {
		caFile, err := ExpandHome(c.CAFile)
		if err != nil {
			return errPath(fieldCAFile, err)
		}

		if _, err := network.CAPool(caFile); err != nil {
			return errPath(fieldCAFile, err)
		}
	}

	if c.CertFile == "" {
		return nil
	}

	certFile, err := ExpandHome(c.CertFile)
	if err != nil {
		return errPath(fieldCertFile, err)
	}

	keyFile, err := ExpandHome(c.KeyFile)
	if err != nil {
		return errPath(fieldKeyFile, err)
	}

	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return errPath(fieldCertFile, err)
	}

	keyPEM, err := os.ReadFile(keyFile)
	if err != nil {
		return errPath(fieldKeyFile, err)
	}

	// Unattributed: a mismatched pair or malformed PEM here could be either
	// file's fault, and both read cleanly above, so blaming certFile
	// specifically would mislead as often as it'd help.
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		return err
	}

	return nil
}

// TLSConfig resolves the block into a *tls.Config, or nil when the block is
// absent, meaning the connection stays plaintext.
func (c *TLSClientConfig) TLSConfig() (*tls.Config, error) {
	if c == nil {
		return nil, nil
	}

	opts := network.ClientTLS{
		ServerName:         c.ServerName,
		MinVersion:         c.MinVersion,
		InsecureSkipVerify: c.InsecureSkipVerify,
	}

	for _, f := range []struct {
		dst *string
		src string
	}{
		{&opts.CAFile, c.CAFile},
		{&opts.CertFile, c.CertFile},
		{&opts.KeyFile, c.KeyFile},
	} {
		if f.src == "" {
			continue
		}

		expanded, err := ExpandHome(f.src)
		if err != nil {
			return nil, err
		}

		*f.dst = expanded
	}

	return network.BuildClientTLS(opts)
}
