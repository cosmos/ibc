// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package config_test

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/internal/config"
)

func TestChainRPCIPC(t *testing.T) {
	// Keep the socket path below the platform's Unix socket length limit.
	dir, err := os.MkdirTemp("", "ibc-ipc-")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(dir)) })
	t.Chdir(dir)
	path := filepath.Join(dir, "geth.ipc")
	listener, err := net.Listen("unix", path)
	require.NoError(t, err)
	server := rpc.NewServer()
	require.NoError(t, server.RegisterName("eth", ipcChain{}))
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = server.ServeListener(listener)
	}()
	t.Cleanup(func() {
		require.NoError(t, listener.Close())
		server.Stop()
		<-done
	})

	for _, endpoint := range []string{path, "./geth.ipc", "geth.ipc"} {
		t.Run(endpoint, func(t *testing.T) {
			cfg := config.DefaultConfig()
			cfg.Chains = config.Chains{{ChainID: "1", EVM: &config.EVMChainConfig{RPC: endpoint}}}
			configPath := filepath.Join(dir, "ibc.yml")
			require.NoError(t, cfg.StoreToFile(configPath))
			loaded, loadErr := config.LoadFromFile(configPath, true)
			require.NoError(t, loadErr)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			client, dialErr := ethclient.DialContext(ctx, loaded.Chains[0].EVM.RPC)
			require.NoError(t, dialErr)
			defer client.Close()
			height, callErr := client.BlockNumber(ctx)
			require.NoError(t, callErr)
			require.Equal(t, uint64(42), height)
		})
	}
}

type ipcChain struct{}

func (ipcChain) BlockNumber() hexutil.Uint64 { return 42 }
