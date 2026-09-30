// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRemoteParamsRejectURLSuffixes(t *testing.T) {
	for _, suffix := range []string{"?token=abc", "?", "#section", "#", "/grpc?token=abc", "/grpc#section"} {
		t.Run(suffix, func(t *testing.T) {
			err := (RemoteParams{URL: "https://prover.example.com" + suffix}).Validate()
			require.ErrorContains(t, err, "url: must not contain")
		})
	}
	for _, path := range []string{"/grpc", "/grpc/", "/grpc%3Fv1", "/grpc%23v1"} {
		require.NoError(t, (RemoteParams{URL: "https://prover.example.com" + path}).Validate())
	}
}

func TestEVMEndpointValidation(t *testing.T) {
	for _, tt := range []struct {
		name, rpc, ws, errContains string
	}{
		{name: "http", rpc: "http://localhost:8545"},
		{name: "https with auth and query", rpc: "https://user:pass@rpc.example.com/v1?token=abc"},
		{name: "websocket rpc", rpc: "wss://rpc.example.com/v1?token=abc"},
		{name: "websocket", rpc: "http://localhost:8545", ws: "ws://localhost:8546"},
		{name: "secure websocket", rpc: "https://rpc.example.com", ws: "wss://rpc.example.com/ws?token=abc"},
		{name: "ipv6", rpc: "http://[::1]:8545", ws: "ws://[::1]:8546"},
		{name: "empty rpc", errContains: "rpc: required"},
		{name: "absolute ipc", rpc: "/tmp/geth.ipc"},
		{name: "relative ipc", rpc: "./geth.ipc"},
		{name: "parent relative ipc", rpc: "../node/geth.ipc"},
		{name: "bare ipc filename", rpc: "geth.ipc"},
		{name: "ipc without extension", rpc: "node-socket"},
		{name: "windows named pipe", rpc: `\\.\pipe\geth.ipc`},
		{name: "stdio transport", rpc: "stdio:"},
		{name: "rpc without host", rpc: "https://", errContains: "rpc:"},
		{name: "rpc path without host", rpc: "https:///rpc", errContains: "rpc:"},
		{name: "unsupported rpc", rpc: "ftp://rpc.example.com", errContains: "rpc:"},
		{name: "unsupported ipc scheme", rpc: "ipc:///tmp/geth.ipc", errContains: "rpc:"},
		{name: "rpc invalid port", rpc: "http://localhost:abc", errContains: "rpc:"},
		{name: "rpc port out of range", rpc: "http://localhost:65536", errContains: "rpc:"},
		{name: "rpc empty port", rpc: "http://localhost:", errContains: "rpc:"},
		{name: "rpc fragment", rpc: "https://rpc.example.com/#fragment", errContains: "rpc:"},
		{name: "ws without host", rpc: "http://localhost", ws: "wss://", errContains: "ws:"},
		{name: "ws path without host", rpc: "http://localhost", ws: "ws:///path", errContains: "ws:"},
		{name: "wrong ws scheme", rpc: "http://localhost", ws: "https://localhost", errContains: "ws:"},
		{name: "ipc is not a websocket", rpc: "/tmp/geth.ipc", ws: "/tmp/geth.ipc", errContains: "ws:"},
		{name: "ws invalid port", rpc: "http://localhost", ws: "wss://localhost:abc", errContains: "ws:"},
		{name: "ws port out of range", rpc: "http://localhost", ws: "wss://localhost:65536", errContains: "ws:"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := (EVMChainConfig{RPC: tt.rpc, WS: tt.ws}).Validate(false)
			if tt.errContains == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.errContains)
			}
		})
	}
}
