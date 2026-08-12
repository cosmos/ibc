// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/config"
)

func TestRequireLocalAttestor(t *testing.T) {
	cfg := config.Config{
		Attestors: config.Attestors{
			{Name: "local-attestor", Type: config.AttestorTypeLocal},
			{Name: "remote-attestor", Type: config.AttestorTypeRemote, GRPC: "attestor.example.com:3000"},
		},
	}

	require.NoError(t, requireLocalAttestor(cfg, "local-attestor"))

	// a remote attestor's process isn't this config's own server -- fail
	// clearly rather than silently dialing the wrong thing
	require.ErrorContains(t, requireLocalAttestor(cfg, "remote-attestor"), `attestor "remote-attestor" is not local`)

	// unknown name
	require.ErrorContains(t, requireLocalAttestor(cfg, "nonexistent"), `attestor "nonexistent" not found`)
}

func TestDialableAddress(t *testing.T) {
	require.Equal(t, "127.0.0.1:3000", dialableAddress("0.0.0.0:3000"))
	require.Equal(t, "127.0.0.1:3000", dialableAddress("[::]:3000"))
	require.Equal(t, "attestor.example.com:3000", dialableAddress("attestor.example.com:3000"))
	require.Equal(t, "127.0.0.1:3000", dialableAddress("127.0.0.1:3000"))
	// malformed input passes through unchanged rather than panicking
	require.Equal(t, "not-a-host-port", dialableAddress("not-a-host-port"))
}
