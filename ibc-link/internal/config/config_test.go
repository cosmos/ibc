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
					c.GRPC.ListenAddress = "0.0.0.0:8080"
				},
			},
			{
				name: "invalid listen address",
				patch: func(c *Config) {
					c.GRPC.ListenAddress = "invalid"
				},
				errContains: "expected address in host:port",
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
grpc:
  listenAddr: 127.0.0.1:9090
`)

			// ACT
			config, err := LoadFromFile(path, true)

			// ASSERT
			require.NoError(t, err)
			assert.Equal(t, "127.0.0.1:9090", config.GRPC.ListenAddress)
		})

		t.Run("envSubstitution", func(t *testing.T) {
			// ARRANGE
			t.Setenv("GRPC_PORT", "9091")
			path := writeTestConfig(t, `
grpc:
  listenAddr: 127.0.0.1:${GRPC_PORT}
`)

			// ACT
			config, err := LoadFromFile(path, true)

			// ASSERT
			require.NoError(t, err)
			assert.Equal(t, "127.0.0.1:9091", config.GRPC.ListenAddress)
		})

		t.Run("invalidYaml", func(t *testing.T) {
			// ARRANGE
			path := writeTestConfig(t, `
grpc:
  listenAddr: [
`)

			// ACT
			_, err := LoadFromFile(path, true)

			// ASSERT
			require.Error(t, err)
		})

		t.Run("validationFails", func(t *testing.T) {
			// ARRANGE
			path := writeTestConfig(t, `
grpc:
  listenAddr: invalid
`)

			// ACT
			_, err := LoadFromFile(path, true)

			// ASSERT
			require.ErrorContains(t, err, "validation failed")
			require.ErrorContains(t, err, "expected address in host:port")
		})

		t.Run("fileNotFound", func(t *testing.T) {
			// ARRANGE
			path := filepath.Join(t.TempDir(), "missing.yml")

			// ACT
			_, err := LoadFromFile(path, true)

			// ASSERT
			require.Error(t, err)
		})
	})
}

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "ibc.yml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	return path
}
