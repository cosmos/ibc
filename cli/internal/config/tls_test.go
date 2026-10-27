// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/internal/testutil/certs"
)

func TestTLSClientConfigValidate(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := certs.WriteSelfSigned(t, dir, "test")
	_, otherKey := certs.WriteSelfSigned(t, dir, "other")
	badPEM := filepath.Join(dir, "bad.pem")
	require.NoError(t, os.WriteFile(badPEM, []byte("not PEM"), 0o600))

	for _, tt := range []struct {
		name        string
		cfg         *TLSClientConfig
		errContains string
	}{
		{
			name: "absent block is valid",
			cfg:  nil,
		},
		{
			name: "empty block means system roots",
			cfg:  &TLSClientConfig{},
		},
		{
			name:        "skip verify with CA is rejected",
			cfg:         &TLSClientConfig{CAFile: certFile, InsecureSkipVerify: true},
			errContains: "caFile: must not be set",
		},
		{
			name:        "malformed CA",
			cfg:         &TLSClientConfig{CAFile: badPEM},
			errContains: "no PEM certificates",
		},
		{
			name:        "malformed client certificate",
			cfg:         &TLSClientConfig{CertFile: badPEM, KeyFile: keyFile},
			errContains: "tls: failed to find any PEM data",
		},
		{
			name:        "mismatched key pair",
			cfg:         &TLSClientConfig{CertFile: certFile, KeyFile: otherKey},
			errContains: "tls: private key does not match public key",
		},
		{
			name: "full mTLS block",
			cfg: &TLSClientConfig{
				CAFile:     certFile,
				CertFile:   certFile,
				KeyFile:    keyFile,
				ServerName: "attestor.example.com",
			},
		},
		{
			name:        "cert without key",
			cfg:         &TLSClientConfig{CertFile: certFile},
			errContains: "keyFile: required when certFile is set",
		},
		{
			name:        "key without cert",
			cfg:         &TLSClientConfig{KeyFile: keyFile},
			errContains: "certFile: required when keyFile is set",
		},
		{
			name:        "missing ca file",
			cfg:         &TLSClientConfig{CAFile: filepath.Join(dir, "absent.crt")},
			errContains: "caFile: read CA file",
		},
		{
			name: "missing cert file",
			cfg: &TLSClientConfig{
				CertFile: filepath.Join(dir, "absent.crt"),
				KeyFile:  keyFile,
			},
			errContains: "load client certificate: open " + filepath.Join(dir, "absent.crt"),
		},
		{
			name:        "a directory is not a certificate",
			cfg:         &TLSClientConfig{CAFile: dir},
			errContains: "caFile",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.errContains == "" {
				require.NoError(t, err)
				return
			}

			require.ErrorContains(t, err, tt.errContains)
		})
	}
}

func TestTLSClientConfigTLSConfig(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := certs.WriteSelfSigned(t, dir, "test")

	t.Run("absent block yields no tls config", func(t *testing.T) {
		var cfg *TLSClientConfig

		got, err := cfg.TLSConfig()
		require.NoError(t, err)
		require.Nil(t, got, "an absent block must leave the transport's defaults")
	})

	t.Run("empty block yields a usable config", func(t *testing.T) {
		got, err := (&TLSClientConfig{}).TLSConfig()
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Nil(t, got.RootCAs)
		require.Empty(t, got.Certificates)
		require.False(t, got.InsecureSkipVerify)
	})

	t.Run("client certificate is wired for mTLS", func(t *testing.T) {
		got, err := (&TLSClientConfig{
			CAFile:   certFile,
			CertFile: certFile,
			KeyFile:  keyFile,
		}).TLSConfig()
		require.NoError(t, err)
		require.NotNil(t, got.RootCAs)
		require.Len(t, got.Certificates, 1)
	})

	t.Run("serverName and skip-verify carry through", func(t *testing.T) {
		got, err := (&TLSClientConfig{
			ServerName:         "attestor.example.com",
			InsecureSkipVerify: true,
		}).TLSConfig()
		require.NoError(t, err)
		require.Equal(t, "attestor.example.com", got.ServerName)
		require.True(t, got.InsecureSkipVerify)
	})

	t.Run("a bad file is reported rather than silently dropped", func(t *testing.T) {
		_, err := (&TLSClientConfig{CAFile: filepath.Join(dir, "absent.crt")}).TLSConfig()
		require.Error(t, err)
	})

	// TLSConfig runs at connect time even when validation was skipped, so a
	// lone key must not quietly degrade mTLS to one-way TLS.
	t.Run("a key without a certificate is rejected", func(t *testing.T) {
		_, err := (&TLSClientConfig{KeyFile: keyFile}).TLSConfig()
		require.ErrorContains(t, err, "certFile: required when keyFile is set")
	})

	t.Run("caFile with insecureSkipVerify is rejected", func(t *testing.T) {
		_, err := (&TLSClientConfig{CAFile: certFile, InsecureSkipVerify: true}).TLSConfig()
		require.ErrorContains(t, err, "caFile: must not be set when insecureSkipVerify is enabled")
	})

	// The pair is read once, so an expired one fails validation instead of
	// every handshake.
	t.Run("an expired client certificate is rejected", func(t *testing.T) {
		expiredCert, expiredKey := certs.WriteExpiredSelfSigned(t, dir, "expired")
		err := (&TLSClientConfig{CertFile: expiredCert, KeyFile: expiredKey}).Validate()
		require.ErrorContains(t, err, "client certificate "+expiredCert+" expired")
	})

	t.Run("an expired client certificate is rejected without a parsed leaf", func(t *testing.T) {
		t.Setenv("GODEBUG", "x509keypairleaf=0")
		expiredCert, expiredKey := certs.WriteExpiredSelfSigned(t, dir, "expired-noleaf")
		err := (&TLSClientConfig{CertFile: expiredCert, KeyFile: expiredKey}).Validate()
		require.ErrorContains(t, err, "client certificate "+expiredCert+" expired")
	})

	t.Run("a not yet valid client certificate is rejected", func(t *testing.T) {
		futureCert, futureKey := certs.WriteNotYetValidSelfSigned(t, dir, "future")
		err := (&TLSClientConfig{CertFile: futureCert, KeyFile: futureKey}).Validate()
		require.ErrorContains(t, err, "client certificate "+futureCert+" is not valid until")
	})
}

// Certificate paths go through the same ~ expansion as signer key files.
func TestTLSClientConfigExpandsHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := home

	certFile, keyFile := certs.WriteSelfSigned(t, dir, "test")

	rel := func(path string) string {
		trimmed, relErr := filepath.Rel(home, path)
		require.NoError(t, relErr)

		return "~/" + trimmed
	}

	cfg := &TLSClientConfig{CertFile: rel(certFile), KeyFile: rel(keyFile), CAFile: rel(certFile)}

	require.NoError(t, cfg.Validate())

	got, err := cfg.TLSConfig()
	require.NoError(t, err)
	require.Len(t, got.Certificates, 1)
	require.NotNil(t, got.RootCAs)
}

// The yaml keys are the operator-facing contract, so pin them.
func TestTLSClientConfigYAML(t *testing.T) {
	t.Run("decodes every field", func(t *testing.T) {
		var cfg TLSClientConfig

		raw := `
caFile: /tls/ca.crt
certFile: /tls/client.crt
keyFile: /tls/client.key
serverName: attestor.example.com
insecureSkipVerify: true
`
		require.NoError(t, yaml.UnmarshalWithOptions([]byte(raw), &cfg, yaml.DisallowUnknownField()))
		require.Equal(t, TLSClientConfig{
			CAFile:             "/tls/ca.crt",
			CertFile:           "/tls/client.crt",
			KeyFile:            "/tls/client.key",
			ServerName:         "attestor.example.com",
			InsecureSkipVerify: true,
		}, cfg)
	})

	t.Run("rejects an unknown field", func(t *testing.T) {
		var cfg TLSClientConfig

		err := yaml.UnmarshalWithOptions([]byte("caCert: /tls/ca.crt\n"), &cfg, yaml.DisallowUnknownField())
		require.Error(t, err, "a misspelled key must not be silently ignored")
	})

	t.Run("an empty block is a present block", func(t *testing.T) {
		var attestor AttestorConfig

		require.NoError(t, yaml.Unmarshal([]byte("name: a\ntype: remote\ngrpc: host:3000\ntls: {}\n"), &attestor))
		require.NotNil(t, attestor.TLS, "an empty tls block decodes as present")
	})

	t.Run("omitted block stays nil", func(t *testing.T) {
		var attestor AttestorConfig

		require.NoError(t, yaml.Unmarshal([]byte("name: a\ntype: remote\ngrpc: host:3000\n"), &attestor))
		require.Nil(t, attestor.TLS)
	})
}

func TestAttestorConfigTLS(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := certs.WriteSelfSigned(t, dir, "test")

	t.Run("remote attestor accepts a tls block", func(t *testing.T) {
		cfg := AttestorConfig{
			Name: "attestor-1",
			Type: AttestorTypeRemote,
			GRPC: "https://attestor.example.com:3000",
			TLS:  &TLSClientConfig{CertFile: certFile, KeyFile: keyFile},
		}
		require.NoError(t, cfg.Validate())
	})

	t.Run("remote attestor rejects a bad tls block", func(t *testing.T) {
		cfg := AttestorConfig{
			Name: "attestor-1",
			Type: AttestorTypeRemote,
			GRPC: "https://attestor.example.com:3000",
			TLS:  &TLSClientConfig{CertFile: certFile},
		}
		require.ErrorContains(t, cfg.Validate(), "tls.keyFile")
	})

	t.Run("remote attestor rejects a malformed URL", func(t *testing.T) {
		for grpc, want := range map[string]string{
			"https://attestor.example.com?token=abc":      "grpc: must not have a query or fragment",
			"https://attestor.example.com/grpc?token=abc": "grpc: must not have a query or fragment",
			"https://attestor.example.com?":               "grpc: must not have a query or fragment",
			"https://attestor.example.com#section":        "grpc: must not have a query or fragment",
			"https://attestor.example.com#":               "grpc: must not have a query or fragment",
			"https://":                                    "grpc: must include a host",
			"https://attestor.example.com:abc":            "grpc: invalid URL",
			"attestor.example.com:abc":                    "grpc: invalid URL",
			"attestor.example.com:3000?token=abc":         "grpc: must not have a query or fragment",
		} {
			cfg := AttestorConfig{Name: "attestor-1", Type: AttestorTypeRemote, GRPC: grpc}
			require.ErrorContains(t, cfg.Validate(), want, grpc)
		}

		for _, grpc := range []string{
			"https://attestor.example.com",
			"https://attestor.example.com/grpc",
			"https://user:secret@attestor.example.com",
			"attestor.example.com:3000",
		} {
			cfg := AttestorConfig{Name: "attestor-1", Type: AttestorTypeRemote, GRPC: grpc}
			require.NoError(t, cfg.Validate(), grpc)
		}
	})

	t.Run("remote attestor rejects a tls block on a plaintext address", func(t *testing.T) {
		for _, grpc := range []string{"attestor.example.com:3000", "http://attestor.example.com:3000"} {
			cfg := AttestorConfig{Name: "attestor-1", Type: AttestorTypeRemote, GRPC: grpc, TLS: &TLSClientConfig{}}
			require.ErrorContains(t, cfg.Validate(), "tls: requires an https:// address", grpc)
		}
	})

	t.Run("local attestor rejects a tls block", func(t *testing.T) {
		cfg := AttestorConfig{
			Name:    "attestor-1",
			Type:    AttestorTypeLocal,
			ChainID: "1",
			Signer:  "signer-1",
			TLS:     &TLSClientConfig{},
		}
		require.ErrorContains(t, cfg.Validate(), "tls: must not be set for local attestors")
	})
}

func TestSignerConfigTLS(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := certs.WriteSelfSigned(t, dir, "test")

	t.Run("remote signer accepts a tls block", func(t *testing.T) {
		cfg := SignerConfig{
			Alias:       "kms",
			Type:        SignerRemote,
			GRPC:        "https://kms.example.com:9090",
			RemoteKeyID: "key-1",
			TLS:         &TLSClientConfig{CAFile: certFile, CertFile: certFile, KeyFile: keyFile},
		}
		require.NoError(t, cfg.Validate())
	})

	t.Run("remote signer rejects a malformed URL", func(t *testing.T) {
		for grpc, want := range map[string]string{
			"https://kms.example.com/kms":         "grpc: must not have a path",
			"http://kms.example.com/kms":          "grpc: must not have a path",
			"https://kms.example.com?x=1":         "grpc: must not have a query or fragment",
			"https://kms.example.com#x":           "grpc: must not have a query or fragment",
			"https://":                            "grpc: must include a host",
			"https://kms.example.com:abc":         "grpc: invalid URL",
			"https://user:secret@kms.example.com": "grpc: must not have userinfo",
			"https://token@kms.example.com":       "grpc: must not have userinfo",
		} {
			cfg := SignerConfig{Alias: "kms", Type: SignerRemote, GRPC: grpc, RemoteKeyID: "key-1"}
			require.ErrorContains(t, cfg.Validate(), want, grpc)
		}

		for _, grpc := range []string{"https://kms.example.com", "https://kms.example.com/", "kms.example.com:9090"} {
			cfg := SignerConfig{Alias: "kms", Type: SignerRemote, GRPC: grpc, RemoteKeyID: "key-1"}
			require.NoError(t, cfg.Validate(), grpc)
		}
	})

	t.Run("remote signer rejects a tls block on a plaintext address", func(t *testing.T) {
		cfg := SignerConfig{
			Alias:       "kms",
			Type:        SignerRemote,
			GRPC:        "kms.example.com:9090",
			RemoteKeyID: "key-1",
			TLS:         &TLSClientConfig{},
		}
		require.ErrorContains(t, cfg.Validate(), "tls: requires an https:// address")
	})

	t.Run("local signer rejects a tls block", func(t *testing.T) {
		cfg := SignerConfig{
			Alias: "local",
			Type:  SignerLocal,
			File:  certFile,
			TLS:   &TLSClientConfig{},
		}
		require.ErrorContains(t, cfg.Validate(), "tls: must not be set for local signer")
	})
}

func TestRemoteProverConfigValidate(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := certs.WriteSelfSigned(t, dir, "test")

	for _, tt := range []struct {
		name        string
		prover      RemoteProverConfig
		errContains string
	}{
		{
			name:   "https url",
			prover: RemoteProverConfig{URL: "https://prover.example.com:9090"},
		},
		{
			name:   "http url",
			prover: RemoteProverConfig{URL: "http://prover.example.com:9090"},
		},
		{
			name:   "bare host:port",
			prover: RemoteProverConfig{URL: "prover.example.com:9090"},
		},
		{
			name: "https url with tls",
			prover: RemoteProverConfig{
				URL: "https://prover.example.com:9090",
				TLS: &TLSClientConfig{CertFile: certFile, KeyFile: keyFile},
			},
		},
		{
			name:   "uppercase https with tls",
			prover: RemoteProverConfig{URL: "HTTPS://prover.example.com:9090", TLS: &TLSClientConfig{}},
		},
		{
			name:        "non-http scheme",
			prover:      RemoteProverConfig{URL: "grpc://prover.example.com:9090"},
			errContains: "url: must be a bare host:port or an http(s):// URL",
		},
		{
			name:        "empty url",
			prover:      RemoteProverConfig{},
			errContains: "url: required",
		},
		{
			// Its settings would be read and then silently ignored.
			name: "tls block with a plaintext url",
			prover: RemoteProverConfig{
				URL: "http://prover.example.com:9090",
				TLS: &TLSClientConfig{},
			},
			errContains: "tls: requires an https:// address",
		},
		{
			name: "tls block with a bare host:port",
			prover: RemoteProverConfig{
				URL: "prover.example.com:9090",
				TLS: &TLSClientConfig{},
			},
			errContains: "tls: requires an https:// address",
		},
		{
			name:        "query on a url",
			prover:      RemoteProverConfig{URL: "https://prover.example.com/grpc?token=abc"},
			errContains: "url: must not have a query or fragment",
		},
		{
			name:        "fragment on a url",
			prover:      RemoteProverConfig{URL: "http://prover.example.com#section"},
			errContains: "url: must not have a query or fragment",
		},
		{
			name:   "path on a url",
			prover: RemoteProverConfig{URL: "https://prover.example.com/grpc"},
		},
		{
			name:   "userinfo on a url",
			prover: RemoteProverConfig{URL: "https://user:secret@prover.example.com"},
		},
		{
			name:        "url without a host",
			prover:      RemoteProverConfig{URL: "https://"},
			errContains: "url: must include a host",
		},
		{
			name:        "bare host:port with a bad port",
			prover:      RemoteProverConfig{URL: "prover.example.com:abc"},
			errContains: "url: invalid URL",
		},
		{
			name:        "bare host:port with a query",
			prover:      RemoteProverConfig{URL: "prover.example.com:9090?token=abc"},
			errContains: "url: must not have a query or fragment",
		},
		{
			name:        "url with a bad port",
			prover:      RemoteProverConfig{URL: "https://prover.example.com:abc"},
			errContains: "url: invalid URL",
		},
		{
			name: "bad tls block on an https url",
			prover: RemoteProverConfig{
				URL: "https://prover.example.com:9090",
				TLS: &TLSClientConfig{CertFile: certFile},
			},
			errContains: "tls.keyFile",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.prover.Validate()
			if tt.errContains == "" {
				require.NoError(t, err)
				return
			}

			require.ErrorContains(t, err, tt.errContains)
		})
	}
}

// Attestors and provers share endpointURL, so a bare host:port means http://
// for both.
func TestEndpointURL(t *testing.T) {
	for addr, want := range map[string]string{
		"svc.example.com:3000":         "http://svc.example.com:3000",
		"http://svc.example.com:3000":  "http://svc.example.com:3000",
		"https://svc.example.com:3000": "https://svc.example.com:3000",
		"https://svc.example.com/grpc": "https://svc.example.com/grpc",
	} {
		require.Equal(t, want, AttestorConfig{GRPC: addr}.EndpointURL(), addr)
		require.Equal(t, want, RemoteProverConfig{URL: addr}.EndpointURL(), addr)
	}
}

// The scheme decides TLS for every service; the block only customizes it.
func TestResolveEndpoint(t *testing.T) {
	dir := t.TempDir()
	certFile, _ := certs.WriteSelfSigned(t, dir, "test")

	t.Run("https without a block uses default TLS", func(t *testing.T) {
		got, err := ResolveEndpoint("https://svc.example.com", nil, nil)
		require.NoError(t, err)
		require.NotNil(t, got.TLS)
		require.Nil(t, got.TLS.RootCAs)
	})

	t.Run("https with a block uses its settings", func(t *testing.T) {
		got, err := ResolveEndpoint("HTTPS://svc.example.com", &TLSClientConfig{CAFile: certFile}, nil)
		require.NoError(t, err)
		require.NotNil(t, got.TLS.RootCAs)
	})

	t.Run("plaintext addresses use no TLS", func(t *testing.T) {
		for _, addr := range []string{"http://svc.example.com", "svc.example.com:3000", "dns:///svc.example.com"} {
			got, err := ResolveEndpoint(addr, nil, nil)
			require.NoError(t, err)
			require.Nil(t, got.TLS, addr)
			require.Equal(t, addr, got.URL)
		}
	})

	t.Run("a block on a plaintext address is rejected", func(t *testing.T) {
		_, err := ResolveEndpoint("svc.example.com:3000", &TLSClientConfig{}, nil)
		require.ErrorContains(t, err, "tls: requires an https:// address")
	})
}

func TestConfigValidateReportsTLSPath(t *testing.T) {
	dir := t.TempDir()
	certFile, _ := certs.WriteSelfSigned(t, dir, "test")

	cfg := DefaultConfig()
	cfg.Chains = Chains{{ChainID: "1", EVM: &EVMChainConfig{RPC: "http://localhost:8545"}}}
	cfg.Attestors = Attestors{{
		Name: "attestor-1",
		Type: AttestorTypeRemote,
		GRPC: "https://attestor.example.com:3000",
		TLS:  &TLSClientConfig{CAFile: filepath.Join(dir, "absent.crt")},
	}}
	cfg.Signers = Signers{{
		Alias:       "kms",
		Type:        SignerRemote,
		GRPC:        "https://kms.example.com:9090",
		RemoteKeyID: "key-1",
		TLS:         &TLSClientConfig{CAFile: certFile},
	}}

	require.ErrorContains(t, cfg.Validate(), "attestors[0].tls.caFile")
}
