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
attestor:
  attestations:
    - chainId: chain-a
      name: attestation-a
      signer: signer-a
`)

			// ACT
			config, err := LoadFromFile(path, true, true)

			// ASSERT
			require.NoError(t, err)
			require.Len(t, config.Attestor.Attestations, 1)
			assert.Equal(t, "signer-a", config.Attestor.Attestations[0].Signer)
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

func TestAttestorConfigValidate(t *testing.T) {
	for _, tt := range []struct {
		name        string
		attestor    AttestorConfig
		errContains string
	}{
		{
			name:     "empty attestations",
			attestor: AttestorConfig{},
		},
		{
			name: "valid attestations",
			attestor: AttestorConfig{
				Attestations: []AttestationConfig{
					{
						ChainID: "chain-a",
						Name:    "attestation-a",
						Signer:  "signer-a",
					},
					{
						ChainID: "chain-b",
						Name:    "attestation-b",
						Signer:  "signer-b",
					},
				},
			},
		},
		{
			name: "chain id required",
			attestor: AttestorConfig{
				Attestations: []AttestationConfig{{
					Name:   "attestation-a",
					Signer: "signer-a",
				}},
			},
			errContains: ".attestations[0]: .chainId required",
		},
		{
			name: "name required",
			attestor: AttestorConfig{
				Attestations: []AttestationConfig{{
					ChainID: "chain-a",
					Signer:  "signer-a",
				}},
			},
			errContains: ".attestations[0]: .name required",
		},
		{
			name: "signer required",
			attestor: AttestorConfig{
				Attestations: []AttestationConfig{{
					ChainID: "chain-a",
					Name:    "attestation-a",
				}},
			},
			errContains: ".attestations[0]: .signer required",
		},
		{
			name: "duplicate name",
			attestor: AttestorConfig{
				Attestations: []AttestationConfig{
					{
						ChainID: "chain-a",
						Name:    "same",
						Signer:  "signer-a",
					},
					{
						ChainID: "chain-b",
						Name:    "same",
						Signer:  "signer-b",
					},
				},
			},
			errContains: `.attestations[1] duplicate name: "same"`,
		},
		{
			name: "duplicate signer",
			attestor: AttestorConfig{
				Attestations: []AttestationConfig{
					{
						ChainID: "chain-a",
						Name:    "attestation-a",
						Signer:  "same",
					},
					{
						ChainID: "chain-b",
						Name:    "attestation-b",
						Signer:  "same",
					},
				},
			},
			errContains: `.attestations[1] duplicate signer: "same"`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// ACT
			err := tt.attestor.Validate()

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
				Type:  signerTypeLocal,
				File:  keyFile,
			}},
		},
		{
			name: "valid remote",
			signers: Signers{{
				Alias:       "remote",
				Type:        signerTypeRemote,
				GRPC:        "https://kms.example.com",
				RemoteKeyID: "key-1",
			}},
		},
		{
			name: "alias required",
			signers: Signers{{
				Type: signerTypeLocal,
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
				Type:  signerTypeLocal,
			}},
			errContains: ".file required",
		},
		{
			name: "local file must exist",
			signers: Signers{{
				Alias: "local",
				Type:  signerTypeLocal,
				File:  filepath.Join(t.TempDir(), "missing.json"),
			}},
			errContains: ".file",
		},
		{
			name: "remote grpc required",
			signers: Signers{{
				Alias:       "remote",
				Type:        signerTypeRemote,
				RemoteKeyID: "key-1",
			}},
			errContains: ".grpc required",
		},
		{
			name: "remote key id required",
			signers: Signers{{
				Alias: "remote",
				Type:  signerTypeRemote,
				GRPC:  "https://kms.example.com",
			}},
			errContains: ".remoteKeyId required",
		},
		{
			name: "duplicate alias",
			signers: Signers{
				{
					Alias: "same",
					Type:  signerTypeLocal,
					File:  keyFile,
				},
				{
					Alias:       "same",
					Type:        signerTypeRemote,
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

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "ibc.yml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	return path
}
