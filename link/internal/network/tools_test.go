package network

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateListenAddr(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		for _, tt := range []struct {
			name string
			raw  string
		}{
			{
				name: "empty host",
				raw:  ":3000",
			},
			{
				name: "ipv4 host",
				raw:  "127.0.0.1:3000",
			},
			{
				name: "ipv6 host",
				raw:  "[::1]:3000",
			},
			{
				name: "localhost",
				raw:  "localhost:3000",
			},
			{
				name: "service hostname",
				raw:  "my-service:3000",
			},
			{
				name: "fqdn",
				raw:  "api.example.com:3000",
			},
			{
				name: "host delegated to network stack",
				raw:  "my_service:3000",
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				// ARRANGE
				raw := tt.raw

				// ACT
				err := ValidateListenAddr(raw)

				// ASSERT
				require.NoError(t, err)
			})
		}
	})

	t.Run("invalid", func(t *testing.T) {
		for _, tt := range []struct {
			name        string
			raw         string
			errContains string
		}{
			{
				name:        "empty string",
				raw:         "",
				errContains: "empty string provided",
			},
			{
				name:        "missing port",
				raw:         "localhost",
				errContains: "expected address in host:port",
			},
			{
				name:        "non numeric port",
				raw:         "localhost:http",
				errContains: "port must be numeric",
			},
			{
				name:        "port out of range",
				raw:         "localhost:65536",
				errContains: "port must be between 0 and 65535",
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				// ARRANGE
				raw := tt.raw

				// ACT
				err := ValidateListenAddr(raw)

				// ASSERT
				require.ErrorContains(t, err, tt.errContains)
			})
		}
	})
}
