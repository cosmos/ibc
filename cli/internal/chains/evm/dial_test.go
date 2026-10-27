// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type chainIDTestAPI struct {
	result string
	err    error
}

func (a chainIDTestAPI) ChainId() (string, error) { //nolint:revive // JSON-RPC requires eth_chainId.
	return a.result, a.err
}

func chainIDTestServer(t *testing.T, result string, replyErr error, websocket bool) string {
	t.Helper()
	srv := rpc.NewServer()
	t.Cleanup(srv.Stop)
	require.NoError(t, srv.RegisterName("eth", chainIDTestAPI{result: result, err: replyErr}))
	var handler http.Handler = srv
	if websocket {
		handler = srv.WebsocketHandler([]string{"*"})
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	if websocket {
		return "ws" + strings.TrimPrefix(server.URL, "http")
	}
	return server.URL
}

func TestNewValidatesChainID(t *testing.T) {
	for _, transport := range []string{"http", "websocket"} {
		t.Run(transport, func(t *testing.T) {
			for _, tt := range []struct {
				name    string
				result  string
				err     error
				wantErr string
			}{
				{"matching", "0x1", nil, ""},
				{"wrong network", "0x2105", nil, "rpc reports chain ID 8453, configured chain ID is 1"},
				{"RPC error", "", errors.New("chain ID unavailable"), "chain ID unavailable"},
				{"malformed response", "not-a-chain-id", nil, "querying chain ID"},
			} {
				t.Run(tt.name, func(t *testing.T) {
					endpoint := chainIDTestServer(t, tt.result, tt.err, transport == "websocket")
					rpcURL, wsURL := endpoint, ""
					if transport == "websocket" {
						rpcURL = chainIDTestServer(t, "0x1", nil, false)
						wsURL = endpoint
					}

					client, err := New(chainIDEth, rpcURL, wsURL, routerAddress)

					if tt.wantErr != "" {
						require.ErrorContains(t, err, tt.wantErr)
						assert.Nil(t, client)
						return
					}
					require.NoError(t, err)
					assert.Equal(t, chainIDEth, client.ChainID())
				})
			}
		})
	}
}

func TestDialReusesConnectionsUnderConcurrency(t *testing.T) {
	// ARRANGE
	const concurrency = 20
	var opened atomic.Int64
	var inFlight sync.WaitGroup
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Method == "eth_chainId" {
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": "0x1"})
			return
		}
		// Hold every request until the whole round is in flight, so each
		// needs its own connection.
		inFlight.Done()
		inFlight.Wait()
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": "0x0"})
	}))
	srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			opened.Add(1)
		}
	}
	srv.Start()
	t.Cleanup(srv.Close)
	client, err := Dial(chainIDEth, srv.URL)
	require.NoError(t, err)

	round := func() {
		inFlight.Add(concurrency)
		var calls sync.WaitGroup
		for range concurrency {
			calls.Go(func() {
				_, err := client.PendingNonceAt(t.Context(), common.Address{})
				assert.NoError(t, err)
			})
		}
		calls.Wait()
	}

	// ACT
	round()
	round()

	// ASSERT
	assert.Equal(t, int64(concurrency), opened.Load(), "the second round reuses the first round's connections")
}
