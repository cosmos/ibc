// SPDX-License-Identifier: Apache-2.0

package config

import (
	"crypto/tls"
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
			name: "absent block is plaintext and valid",
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
				MinVersion: "1.3",
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
			name:        "tls 1.1 is rejected",
			cfg:         &TLSClientConfig{MinVersion: "1.1"},
			errContains: "minVersion",
		},
		{
			name:        "missing ca file",
			cfg:         &TLSClientConfig{CAFile: filepath.Join(dir, "absent.crt")},
			errContains: "caFile",
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
		require.Nil(t, got, "an absent block must leave the connection plaintext")
	})

	t.Run("empty block yields a usable config", func(t *testing.T) {
		got, err := (&TLSClientConfig{}).TLSConfig()
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, uint16(tls.VersionTLS12), got.MinVersion)
		require.Nil(t, got.GetClientCertificate)
	})

	t.Run("client certificate is wired for mTLS", func(t *testing.T) {
		got, err := (&TLSClientConfig{
			CertFile:   certFile,
			KeyFile:    keyFile,
			MinVersion: "1.3",
		}).TLSConfig()
		require.NoError(t, err)
		require.Equal(t, uint16(tls.VersionTLS13), got.MinVersion)
		require.NotNil(t, got.GetClientCertificate)
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
}

// Certificate paths go through the same ~ expansion as signer key files.
func TestTLSClientConfigExpandsHome(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	dir, err := os.MkdirTemp(home, ".ibc-tls-test-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

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
	require.NotNil(t, got.GetClientCertificate)
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
minVersion: "1.3"
insecureSkipVerify: true
`
		require.NoError(t, yaml.UnmarshalWithOptions([]byte(raw), &cfg, yaml.DisallowUnknownField()))
		require.Equal(t, TLSClientConfig{
			CAFile:             "/tls/ca.crt",
			CertFile:           "/tls/client.crt",
			KeyFile:            "/tls/client.key",
			ServerName:         "attestor.example.com",
			MinVersion:         "1.3",
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
		require.NotNil(t, attestor.TLS, "an empty tls block still enables TLS")
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
			GRPC: "attestor.example.com:3000",
			TLS:  &TLSClientConfig{CertFile: certFile, KeyFile: keyFile},
		}
		require.NoError(t, cfg.Validate())
	})

	t.Run("remote attestor rejects a bad tls block", func(t *testing.T) {
		cfg := AttestorConfig{
			Name: "attestor-1",
			Type: AttestorTypeRemote,
			GRPC: "attestor.example.com:3000",
			TLS:  &TLSClientConfig{CertFile: certFile},
		}
		require.ErrorContains(t, cfg.Validate(), "tls.keyFile")
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
			GRPC:        "kms.example.com:9090",
			RemoteKeyID: "key-1",
			TLS:         &TLSClientConfig{CAFile: certFile, CertFile: certFile, KeyFile: keyFile},
		}
		require.NoError(t, cfg.Validate())
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

func TestRemoteParamsValidate(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := certs.WriteSelfSigned(t, dir, "test")

	for _, tt := range []struct {
		name        string
		params      RemoteParams
		errContains string
	}{
		{
			name:   "https url",
			params: RemoteParams{URL: "https://prover.example.com:9090"},
		},
		{
			name:   "http url",
			params: RemoteParams{URL: "http://prover.example.com:9090"},
		},
		{
			name:   "bare host:port",
			params: RemoteParams{URL: "prover.example.com:9090"},
		},
		{
			name: "https url with tls",
			params: RemoteParams{
				URL: "https://prover.example.com:9090",
				TLS: &TLSClientConfig{CertFile: certFile, KeyFile: keyFile},
			},
		},
		{
			name:   "uppercase https with tls",
			params: RemoteParams{URL: "HTTPS://prover.example.com:9090", TLS: &TLSClientConfig{}},
		},
		{
			name:        "unsupported scheme",
			params:      RemoteParams{URL: "ftp://prover.example.com"},
			errContains: "url: must start with",
		},
		{
			name:        "missing host",
			params:      RemoteParams{URL: "https:///path"},
			errContains: "url: must start with",
		},
		{
			name:        "empty url",
			params:      RemoteParams{},
			errContains: "url: required",
		},
		{
			// tls is a new field, so requiring https rejects nothing that
			// previously loaded, and catches a block that would otherwise be
			// read and then silently ignored.
			name: "tls block with a plaintext url",
			params: RemoteParams{
				URL: "http://prover.example.com:9090",
				TLS: &TLSClientConfig{},
			},
			errContains: "tls: requires an https:// url",
		},
		{
			name: "tls block with a bare host:port",
			params: RemoteParams{
				URL: "prover.example.com:9090",
				TLS: &TLSClientConfig{},
			},
		},
		{
			name:        "bare host:port with a bad port",
			params:      RemoteParams{URL: "prover.example.com:abc"},
			errContains: "url: invalid URL",
		},
		{
			name: "bad tls block on an https url",
			params: RemoteParams{
				URL: "https://prover.example.com:9090",
				TLS: &TLSClientConfig{MinVersion: "1.1"},
			},
			errContains: "tls.minVersion",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.params.Validate()
			if tt.errContains == "" {
				require.NoError(t, err)
				return
			}

			require.ErrorContains(t, err, tt.errContains)
		})
	}
}

func TestRemoteParamsEndpointURL(t *testing.T) {
	for _, tt := range []struct {
		params RemoteParams
		want   string
	}{
		{RemoteParams{URL: "prover.example.com:9090"}, "http://prover.example.com:9090"},
		{RemoteParams{URL: "prover.example.com:9090", TLS: &TLSClientConfig{}}, "https://prover.example.com:9090"},
		{RemoteParams{URL: "http://prover.example.com:9090"}, "http://prover.example.com:9090"},
		{RemoteParams{URL: "https://prover.example.com:9090", TLS: &TLSClientConfig{}}, "https://prover.example.com:9090"},
	} {
		require.Equal(t, tt.want, tt.params.EndpointURL())
	}
}

// A tls error must surface with its full config path so an operator can find
// the offending block.
func TestLoadFromFileRejectsNullTLS(t *testing.T) {
	for _, tt := range []struct {
		name        string
		yaml        string
		errContains string
	}{
		{
			name: "bare block",
			yaml: `
attestors:
  - name: attestor-1
    type: remote
    grpc: attestor.example.com:3000
    tls:
`,
			errContains: "attestors[0].tls: empty",
		},
		{
			name: "every field commented out",
			yaml: `
signers:
  - alias: kms
    type: remote
    grpc: kms.example.com:9090
    remoteKeyId: key-1
    tls:
      # caFile: /etc/ibc/tls/ca.crt
`,
			errContains: "signers[0].tls: empty",
		},
		{
			name: "remote prover params",
			yaml: `
relayer:
  connections:
    - alias: conn
      clientA:
        type: remote
        params:
          url: https://prover.example.com
          tls: null
`,
			errContains: "relayer.connections[0].clientA.params.tls: empty",
		},
		{
			name: "empty mapping is TLS with defaults",
			yaml: `
attestors:
  - name: attestor-1
    type: remote
    grpc: attestor.example.com:3000
    tls: {}
`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadFromFile(writeTestConfig(t, tt.yaml), false)
			if tt.errContains == "" {
				require.NoError(t, err)
				return
			}

			require.ErrorContains(t, err, tt.errContains)
		})
	}
}

func TestConfigValidateReportsTLSPath(t *testing.T) {
	dir := t.TempDir()
	certFile, _ := certs.WriteSelfSigned(t, dir, "test")

	cfg := DefaultConfig()
	cfg.Chains = Chains{{ChainID: "1", EVM: &EVMChainConfig{RPC: "http://localhost:8545"}}}
	cfg.Attestors = Attestors{{
		Name: "attestor-1",
		Type: AttestorTypeRemote,
		GRPC: "attestor.example.com:3000",
		TLS:  &TLSClientConfig{CAFile: filepath.Join(dir, "absent.crt")},
	}}
	cfg.Signers = Signers{{
		Alias:       "kms",
		Type:        SignerRemote,
		GRPC:        "kms.example.com:9090",
		RemoteKeyID: "key-1",
		TLS:         &TLSClientConfig{CAFile: certFile},
	}}

	require.ErrorContains(t, cfg.Validate(), "attestors[0].tls.caFile")
}
