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
			path := writeTestConfig(t, `
server:
  listenAddr: 127.0.0.1:9090
db:
  type: postgres
  url: postgres://user:pass@localhost:5432/ibc
`)

			config, err := LoadFromFile(path, true, true)

			require.NoError(t, err)
			assert.Equal(t, "127.0.0.1:9090", config.Server.ListenAddress)
			assert.Equal(t, DBTypePostgres, config.DB.Type)
			assert.Equal(t, "postgres://user:pass@localhost:5432/ibc", config.DB.URL)
		})

		t.Run("envSubstitution", func(t *testing.T) {
			t.Setenv("DATABASE_NAME", "ibc-test")
			path := writeTestConfig(t, `
db:
  type: postgres
  url: postgres://localhost/${DATABASE_NAME}
`)

			config, err := LoadFromFile(path, true, true)

			require.NoError(t, err)
			assert.Equal(t, "postgres://localhost/ibc-test", config.DB.URL)
		})

		t.Run("expansionIsTyped", func(t *testing.T) {
			t.Setenv("SERVER_PORT", "9091")
			path := writeTestConfig(t, `
server:
  listenAddr: 127.0.0.1:${SERVER_PORT}
`)

			config, err := LoadFromFile(path, false, true)

			require.NoError(t, err)
			assert.Equal(t, "127.0.0.1:${SERVER_PORT}", config.Server.ListenAddress)
		})

		t.Run("missingEnvFails", func(t *testing.T) {
			path := writeTestConfig(t, `
db:
  type: postgres
  url: postgres://localhost/${IBC_LINK_TEST_MISSING_DATABASE}
`)

			_, err := LoadFromFile(path, true, true)

			require.ErrorContains(t, err, "expand .db.url")
			require.ErrorContains(t, err, "environment variable IBC_LINK_TEST_MISSING_DATABASE is not set")
		})

		t.Run("invalidYaml", func(t *testing.T) {
			path := writeTestConfig(t, `
server:
  listenAddr: [
`)

			_, err := LoadFromFile(path, true, true)

			require.Error(t, err)
		})

		t.Run("validationFails", func(t *testing.T) {
			path := writeTestConfig(t, `
server:
  listenAddr: invalid
`)

			_, err := LoadFromFile(path, true, true)

			require.ErrorContains(t, err, "validation failed")
			require.ErrorContains(t, err, "expected address in host:port")
		})

		t.Run("fileNotFound", func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "missing.yml")

			_, err := LoadFromFile(path, true, true)

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
					path := writeTestConfig(t, tt.body)

					_, err := LoadFromFile(path, true, true)

					require.ErrorContains(t, err, "unknown field")
				})
			}
		})

		t.Run("unknownFieldsAllowedWhenDisabled", func(t *testing.T) {
			path := writeTestConfig(t, `
server:
  listenAddress: 127.0.0.1:9090
`)

			config, err := LoadFromFile(path, true, false)

			require.NoError(t, err)
			assert.Equal(t, "0.0.0.0:3000", config.Server.ListenAddress)
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
				db, err := DBConfigFromURL(tt.raw)

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

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "ibc.yml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	return path
}
