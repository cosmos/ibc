// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig(t *testing.T) {
	t.Run("Validate", func(t *testing.T) {
		for _, tt := range []struct {
			name        string
			patch       func(c *Config)
			errContains string
		}{
			{
				name: "valid",
				patch: func(c *Config) {
					c.Server.ListenAddress = "0.0.0.0:8080"
				},
			},
			{
				name: "invalid listen address",
				patch: func(c *Config) {
					c.Server.ListenAddress = "invalid"
				},
				errContains: "expected address in host:port",
			},
			{
				name: "invalid db type",
				patch: func(c *Config) {
					c.DB.Type = "mysql"
				},
				errContains: ".type must be one of",
			},
			{
				name: "empty db url",
				patch: func(c *Config) {
					c.DB.URL = ""
				},
				errContains: ".url must not be empty",
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				config := DefaultConfig()
				if tt.patch != nil {
					tt.patch(&config)
				}

				err := config.Validate()
				if tt.errContains != "" {
					require.ErrorContains(t, err, tt.errContains)
					return
				}

				require.NoError(t, err)
			})
		}
	})

	t.Run("LoadFromFile", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			// ARRANGE
			path := writeTestConfig(t, `
server:
  listenAddr: 127.0.0.1:9090
db:
  type: postgres
  url: postgres://user:pass@localhost:5432/ibc
`)

			// ACT
			config, err := LoadFromFile(path, true, true)

			// ASSERT
			require.NoError(t, err)
			assert.Equal(t, "127.0.0.1:9090", config.Server.ListenAddress)
			assert.Equal(t, DBTypePostgres, config.DB.Type)
			assert.Equal(t, "postgres://user:pass@localhost:5432/ibc", config.DB.URL)
		})

		t.Run("envSubstitution", func(t *testing.T) {
			// ARRANGE
			t.Setenv("SERVER_PORT", "9091")
			path := writeTestConfig(t, `
server:
  listenAddr: 127.0.0.1:${SERVER_PORT}
`)

			// ACT
			config, err := LoadFromFile(path, true, true)

			// ASSERT
			require.NoError(t, err)
			assert.Equal(t, "127.0.0.1:9091", config.Server.ListenAddress)
		})

		t.Run("invalidYaml", func(t *testing.T) {
			// ARRANGE
			path := writeTestConfig(t, `
server:
  listenAddr: [
`)

			// ACT
			_, err := LoadFromFile(path, true, true)

			// ASSERT
			require.Error(t, err)
		})

		t.Run("validationFails", func(t *testing.T) {
			// ARRANGE
			path := writeTestConfig(t, `
server:
  listenAddr: invalid
`)

			// ACT
			_, err := LoadFromFile(path, true, true)

			// ASSERT
			require.ErrorContains(t, err, "validation failed")
			require.ErrorContains(t, err, "expected address in host:port")
		})

		t.Run("fileNotFound", func(t *testing.T) {
			// ARRANGE
			path := filepath.Join(t.TempDir(), "missing.yml")

			// ACT
			_, err := LoadFromFile(path, true, true)

			// ASSERT
			require.Error(t, err)
		})

		t.Run("unknownFieldsFail", func(t *testing.T) {
			for _, tt := range []struct {
				name string
				body string
			}{
				{
					name: "top level",
					body: `
unknown: value
server:
  listenAddr: 127.0.0.1:9090
`,
				},
				{
					name: "nested camel case typo",
					body: `
server:
  listenAddress: 127.0.0.1:9090
`,
				},
				{
					name: "nested snake case typo",
					body: `
server:
  listen_addr: 127.0.0.1:9090
`,
				},
			} {
				t.Run(tt.name, func(t *testing.T) {
					// ARRANGE
					path := writeTestConfig(t, tt.body)

					// ACT
					_, err := LoadFromFile(path, true, true)

					// ASSERT
					require.ErrorContains(t, err, "unknown field")
				})
			}
		})

		t.Run("unknownFieldsAllowedWhenDisabled", func(t *testing.T) {
			// ARRANGE
			path := writeTestConfig(t, `
server:
  listenAddress: 127.0.0.1:9090
`)

			// ACT
			config, err := LoadFromFile(path, true, false)

			// ASSERT
			require.NoError(t, err)
			assert.Equal(t, "0.0.0.0:3000", config.Server.ListenAddress)
		})

		t.Run("attestationSigner", func(t *testing.T) {
			// ARRANGE
			path := writeTestConfig(t, `
signers:
  - alias: signer-a
    type: remote
    grpc: https://kms.example.com
    remoteKeyId: key-a
attestors:
  - name: attestation-a
    chainId: chain-a
    type: local
    signer: signer-a
`)

			// ACT
			config, err := LoadFromFile(path, true, true)

			// ASSERT
			require.NoError(t, err)
			require.Len(t, config.Attestors, 1)
			assert.Equal(t, "signer-a", config.Attestors[0].Signer)
		})

		t.Run("unknownAttestationSignerFails", func(t *testing.T) {
			// ARRANGE
			path := writeTestConfig(t, `
signers:
  - alias: signer-a
    type: remote
    grpc: https://kms.example.com
    remoteKeyId: key-a
attestors:
  - name: attestation-a
    chainId: chain-a
    type: local
    signer: missing-signer
`)

			// ACT
			_, err := LoadFromFile(path, true, true)

			// ASSERT
			require.ErrorContains(t, err, `references unknown signer: "missing-signer"`)
		})
	})

	t.Run("DBConfigFromURL", func(t *testing.T) {
		for _, tt := range []struct {
			name        string
			raw         string
			wantType    string
			errContains string
		}{
			{
				name:     "default relative sqlite path",
				raw:      "ibc.sqlite",
				wantType: DBTypeSQLite,
			},
			{
				name:     "relative sqlite path",
				raw:      "./ibc.db",
				wantType: DBTypeSQLite,
			},
			{
				name:     "absolute sqlite path",
				raw:      "/var/lib/ibc/ibc.db",
				wantType: DBTypeSQLite,
			},
			{
				name:     "extensionless sqlite filename",
				raw:      "my-file",
				wantType: DBTypeSQLite,
			},
			{
				name:     "sqlite extension",
				raw:      "file.sqlite",
				wantType: DBTypeSQLite,
			},
			{
				name:     "parent relative sqlite path",
				raw:      "../../some/relative/database.db",
				wantType: DBTypeSQLite,
			},
			{
				name:     "postgres url",
				raw:      "postgres://user:pass@localhost:5432/ibc",
				wantType: DBTypePostgres,
			},
			{
				name:     "postgresql url",
				raw:      "postgresql://user:pass@localhost:5432/ibc",
				wantType: DBTypePostgres,
			},
			{
				name:        "empty",
				raw:         "",
				errContains: ".url must not be empty",
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				// ACT
				db, err := DBConfigFromURL(tt.raw)

				// ASSERT
				if tt.errContains != "" {
					require.ErrorContains(t, err, tt.errContains)
					return
				}

				require.NoError(t, err)
				assert.Equal(t, tt.wantType, db.Type)
			})
		}
	})
}

func TestAttestorsValidate(t *testing.T) {
	for _, tt := range []struct {
		name        string
		attestors   Attestors
		errContains string
	}{
		{
			name:      "empty",
			attestors: Attestors{},
		},
		{
			name: "valid local and remote",
			attestors: Attestors{
				{Name: "attestor-a", ChainID: "chain-a", Type: AttestorTypeLocal, Signer: "signer-a"},
				{Name: "attestor-b", Type: AttestorTypeRemote, GRPC: "attestor-b.example.com:3000"},
			},
		},
		{
			name: "same name across two remotes is valid",
			attestors: Attestors{
				{Name: "watcher", Type: AttestorTypeRemote, GRPC: "watcher-a.example.com:3000"},
				{Name: "watcher", Type: AttestorTypeRemote, GRPC: "watcher-b.example.com:3000"},
			},
		},
		{
			name:        "name required",
			attestors:   Attestors{{ChainID: "chain-a", Type: AttestorTypeLocal, Signer: "signer-a"}},
			errContains: "[0]: .name required",
		},
		{
			name:        "local chain id required",
			attestors:   Attestors{{Name: "attestor-a", Type: AttestorTypeLocal, Signer: "signer-a"}},
			errContains: "[0]: .chainId required for local attestors",
		},
		{
			name:        "invalid type",
			attestors:   Attestors{{Name: "attestor-a", ChainID: "chain-a", Type: "hybrid"}},
			errContains: `unknown attestor type: "hybrid"`,
		},
		{
			name:        "local missing signer",
			attestors:   Attestors{{Name: "attestor-a", ChainID: "chain-a", Type: AttestorTypeLocal}},
			errContains: ".signer required for local attestors",
		},
		{
			name: "local with grpc set",
			attestors: Attestors{{
				Name: "attestor-a", ChainID: "chain-a", Type: AttestorTypeLocal,
				Signer: "signer-a", GRPC: "attestor-a.example.com:3000",
			}},
			errContains: ".grpc must not be set for local attestors",
		},
		{
			name:        "remote missing grpc",
			attestors:   Attestors{{Name: "attestor-a", Type: AttestorTypeRemote}},
			errContains: ".grpc required for remote attestors",
		},
		{
			name: "remote grpc includes a scheme",
			attestors: Attestors{{
				Name: "attestor-a", Type: AttestorTypeRemote,
				GRPC: "https://attestor-a.example.com:443",
			}},
			errContains: ".grpc must be a bare host:port, not a URL",
		},
		{
			name: "remote with chainId set",
			attestors: Attestors{{
				Name: "attestor-a", ChainID: "chain-a", Type: AttestorTypeRemote,
				GRPC: "attestor-a.example.com:3000",
			}},
			errContains: ".chainId must not be set for remote attestors",
		},
		{
			name: "remote with signer set",
			attestors: Attestors{{
				Name: "attestor-a", Type: AttestorTypeRemote,
				GRPC: "attestor-a.example.com:3000", Signer: "signer-a",
			}},
			errContains: ".signer must not be set for remote attestors",
		},
		{
			name: "remote with nonzero finalityOffset",
			attestors: Attestors{{
				Name: "attestor-a", Type: AttestorTypeRemote,
				GRPC: "attestor-a.example.com:3000", FinalityOffset: 1,
			}},
			errContains: ".finalityOffset must not be set for remote attestors",
		},
		{
			name: "duplicate local name",
			attestors: Attestors{
				{Name: "same", ChainID: "chain-a", Type: AttestorTypeLocal, Signer: "signer-a"},
				{Name: "same", ChainID: "chain-b", Type: AttestorTypeLocal, Signer: "signer-b"},
			},
			errContains: `duplicate local attestor name: "same"`,
		},
		{
			name: "duplicate local signer on the same chain",
			attestors: Attestors{
				{Name: "attestor-a", ChainID: "chain-a", Type: AttestorTypeLocal, Signer: "same"},
				{Name: "attestor-b", ChainID: "chain-a", Type: AttestorTypeLocal, Signer: "same"},
			},
			errContains: `duplicate local attestor signer "same" on chain "chain-a"`,
		},
		{
			name: "same local signer across different chains is allowed",
			attestors: Attestors{
				{Name: "attestor-a", ChainID: "chain-a", Type: AttestorTypeLocal, Signer: "same"},
				{Name: "attestor-b", ChainID: "chain-b", Type: AttestorTypeLocal, Signer: "same"},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// ACT
			err := tt.attestors.Validate()

			// ASSERT
			if tt.errContains != "" {
				require.ErrorContains(t, err, tt.errContains)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestSignerConfigValidate(t *testing.T) {
	keyFile := filepath.Join(t.TempDir(), "key.json")
	require.NoError(t, os.WriteFile(keyFile, []byte("{}"), 0o644))

	for _, tt := range []struct {
		name        string
		signers     Signers
		errContains string
	}{
		{
			name: "valid local",
			signers: Signers{{
				Alias: "local",
				Type:  SignerLocal,
				File:  keyFile,
			}},
		},
		{
			name: "valid remote",
			signers: Signers{{
				Alias:       "remote",
				Type:        SignerRemote,
				GRPC:        "https://kms.example.com",
				RemoteKeyID: "key-1",
			}},
		},
		{
			name: "alias required",
			signers: Signers{{
				Type: SignerLocal,
				File: keyFile,
			}},
			errContains: ".alias required",
		},
		{
			name: "type required",
			signers: Signers{{
				Alias: "local",
				File:  keyFile,
			}},
			errContains: ".type required",
		},
		{
			name: "invalid type",
			signers: Signers{{
				Alias: "local",
				Type:  "kms",
			}},
			errContains: ".type must be one of",
		},
		{
			name: "local file required",
			signers: Signers{{
				Alias: "local",
				Type:  SignerLocal,
			}},
			errContains: ".file required",
		},
		{
			name: "local file must exist",
			signers: Signers{{
				Alias: "local",
				Type:  SignerLocal,
				File:  filepath.Join(t.TempDir(), "missing.json"),
			}},
			errContains: ".file",
		},
		{
			name: "remote grpc required",
			signers: Signers{{
				Alias:       "remote",
				Type:        SignerRemote,
				RemoteKeyID: "key-1",
			}},
			errContains: ".grpc required",
		},
		{
			name: "remote key id required",
			signers: Signers{{
				Alias: "remote",
				Type:  SignerRemote,
				GRPC:  "https://kms.example.com",
			}},
			errContains: ".remoteKeyId required",
		},
		{
			name: "duplicate alias",
			signers: Signers{
				{
					Alias: "same",
					Type:  SignerLocal,
					File:  keyFile,
				},
				{
					Alias:       "same",
					Type:        SignerRemote,
					GRPC:        "https://kms.example.com",
					RemoteKeyID: "key-1",
				},
			},
			errContains: ".signers duplicate alias",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.signers.Validate()
			if tt.errContains != "" {
				require.ErrorContains(t, err, tt.errContains)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestSignerConfigValidateRequiresExactLocalFilePath(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "signer.json"), []byte("{}"), 0o600))

	err := (SignerConfig{
		Alias: "local",
		Type:  SignerLocal,
		File:  filepath.Join(dir, "signer"),
	}).Validate()
	require.Error(t, err)
}

func TestChainDeployerCrossValidation(t *testing.T) {
	base := DefaultConfig()
	base.Chains = []ChainConfig{
		{
			ChainID:  "1",
			Deployer: "missing",
			EVM: &EVMChainConfig{
				RPC:         "http://localhost:8545",
				ICS26Router: "0x0000000000000000000000000000000000000001",
			},
		},
	}

	err := base.Validate()
	require.ErrorContains(t, err, "deployer references unknown signer")

	base.Signers = Signers{{
		Alias:       "missing",
		Type:        SignerRemote,
		GRPC:        "localhost:9000",
		RemoteKeyID: "k1",
	}}
	require.NoError(t, base.Validate())
}

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "ibc.yml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	return path
}

func TestConfigAccessors(t *testing.T) {
	cfg := Config{
		Signers: Signers{{Alias: "s1", Type: SignerRemote, GRPC: "g", RemoteKeyID: "k"}},
		Attestors: Attestors{
			{ChainID: "1", Name: "a1", Type: AttestorTypeLocal, Signer: "s1"},
			{ChainID: "2", Name: "a2", Type: AttestorTypeLocal, Signer: "s1"},
			{ChainID: "1", Name: "a3", Type: AttestorTypeLocal, Signer: "s1"},
		},
	}

	signer, ok := cfg.Signer("s1")
	require.True(t, ok)
	require.Equal(t, "g", signer.GRPC)
	_, ok = cfg.Signer("nope")
	require.False(t, ok)

	attestor, ok := cfg.AttestorByName("a2")
	require.True(t, ok)
	require.Equal(t, "2", attestor.ChainID)
	_, ok = cfg.AttestorByName("nope")
	require.False(t, ok)

	forChain := cfg.AttestorsForChain("1")
	require.Len(t, forChain, 2)
	require.Equal(t, "a1", forChain[0].Name)
	require.Equal(t, "a3", forChain[1].Name)
	require.Empty(t, cfg.AttestorsForChain("9"))
}

func TestCollectComments(t *testing.T) {
	cfg := Config{
		Chains: []ChainConfig{
			{ChainID: "1", EVM: &EVMChainConfig{ICS26Router: ""}},
			{ChainID: "2", EVM: &EVMChainConfig{ICS26Router: "0xabc"}},
		},
		Relayer: RelayerConfig{Connections: []ConnectionConfig{
			{ClientA: ClientEnd{ChainID: "1"}, ClientB: ClientEnd{ChainID: "2", Signer: "relayer-key"}},
		}},
		Attestors: Attestors{
			{ChainID: "2", Name: "attestor-2-0xabc", Type: AttestorTypeLocal},
			{ChainID: "1", Name: "attestor-1-0xdef", Type: AttestorTypeLocal, Signer: "watcher"},
			{Name: "remote", Type: AttestorTypeRemote, GRPC: "attestor.example.com:3000"},
		},
	}

	require.Equal(t, map[string]string{
		"$.chains[0].evm.ics26Router":             "TODO: fill in",
		"$.relayer.connections[0].clientA.signer": "TODO: signers[] alias that submits relay txs on chainA",
		"$.attestors[0].signer":                   "TODO: signers[] alias backing this attestor's key",
		"$.attestors[0].finalityOffset":           finalityOffsetTODO,
		"$.attestors[1].finalityOffset":           finalityOffsetTODO,
	}, CollectComments(cfg))

	// nothing left blank -> only the always-on finalityOffset reminder remains
	require.Equal(t, map[string]string{
		"$.attestors[0].finalityOffset": finalityOffsetTODO,
	}, CollectComments(Config{
		Chains: []ChainConfig{{ChainID: "1", EVM: &EVMChainConfig{ICS26Router: "0xabc"}}},
		Relayer: RelayerConfig{Connections: []ConnectionConfig{
			{ClientA: ClientEnd{ChainID: "1", Signer: "a"}, ClientB: ClientEnd{ChainID: "2", Signer: "b"}},
		}},
		Attestors: Attestors{{ChainID: "1", Name: "a", Type: AttestorTypeLocal, Signer: "watcher"}},
	}))
}
