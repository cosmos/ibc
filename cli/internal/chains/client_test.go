// SPDX-License-Identifier: Apache-2.0

package chains

import (
	"context"
	"math/big"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/goccy/go-yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/internal/config"
)

type idleRPC struct {
	chainID        hexutil.Uint64
	headerRequests atomic.Int32
}

func (s *idleRPC) ChainId() hexutil.Uint64 { //nolint:revive // JSON-RPC requires eth_chainId.
	return s.chainID
}

func (s *idleRPC) GetBlockByNumber(context.Context, string, bool) (*types.Header, error) {
	s.headerRequests.Add(1)
	return &types.Header{Number: big.NewInt(100), Difficulty: big.NewInt(0), Time: 1}, nil
}

func TestWaitForChainConfigIsScopedToRPC(t *testing.T) {
	var cfg config.Config
	require.NoError(t, yaml.Unmarshal([]byte(`chains:
  - chainId: "324"
    evm:
      waitForChain: false
  - chainId: "1"
    evm: {}
  - chainId: "2"
    evm:
      waitForChain: true
`), &cfg))

	endpoints := make([]*idleRPC, len(cfg.Chains))
	for i := range cfg.Chains {
		chain := &cfg.Chains[i]
		chainID, err := strconv.ParseUint(chain.ChainID, 10, 64)
		require.NoError(t, err)
		endpoint := &idleRPC{chainID: hexutil.Uint64(chainID)}
		// Each configured chain has a separate RPC connection.
		srv := rpc.NewServer()
		require.NoError(t, srv.RegisterName("eth", endpoint))
		t.Cleanup(srv.Stop)
		httpServer := httptest.NewServer(srv)
		t.Cleanup(httpServer.Close)
		chain.EVM.RPC = httpServer.URL
		chain.EVM.ICS26Router = "0x0000000000000000000000000000000000000001"
		endpoints[i] = endpoint
	}

	clients, err := NewClientSetFromConfig(cfg)
	require.NoError(t, err)
	for i, chain := range cfg.Chains {
		t.Run(chain.ChainID, func(t *testing.T) {
			client, ok := clients.Get(chain.ChainID)
			require.True(t, ok)
			ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
			defer cancel()
			err := client.WaitForChain(ctx)
			if chain.ChainID == "324" {
				require.NoError(t, err)
				assert.Zero(t, endpoints[i].headerRequests.Load())
			} else {
				require.ErrorIs(t, err, context.DeadlineExceeded)
				assert.Positive(t, endpoints[i].headerRequests.Load())
			}
		})
	}
}
