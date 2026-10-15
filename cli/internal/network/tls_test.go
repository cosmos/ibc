// SPDX-License-Identifier: Apache-2.0

package network

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEndpointGRPCTarget(t *testing.T) {
	for url, want := range map[string]string{
		"https://kms.example.com:9090": "kms.example.com:9090",
		"HTTPS://kms.example.com:9090": "kms.example.com:9090",
		"https://kms.example.com":      "kms.example.com:443",
		"http://kms.example.com":       "kms.example.com:80",
		"https://[::1]":                "[::1]:443",
		"kms.example.com:9090":         "kms.example.com:9090",
		"kms.example.com":              "kms.example.com",
		"dns:///kms.example.com:9090":  "dns:///kms.example.com:9090",
		"unix:///run/kms.sock":         "unix:///run/kms.sock",
	} {
		require.Equal(t, want, Endpoint{URL: url}.GRPCTarget(), url)
	}
}
