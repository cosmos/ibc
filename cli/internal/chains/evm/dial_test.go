// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDialReusesConnectionsUnderConcurrency(t *testing.T) {
	// ARRANGE
	const concurrency = 20
	var opened atomic.Int64
	var inFlight sync.WaitGroup
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID json.RawMessage `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
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
