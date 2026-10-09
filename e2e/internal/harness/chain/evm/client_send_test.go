// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// reply is one scripted answer: a result, a node error, or a dropped
// connection.
type reply struct {
	result  any
	nodeErr string
	drop    bool
}

// fakeNode answers JSON-RPC requests from per-method scripts; a script's last
// reply repeats.
type fakeNode struct {
	mu      sync.Mutex
	scripts map[string][]reply
	calls   map[string]int
}

func (n *fakeNode) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	n.mu.Lock()
	script := n.scripts[req.Method]
	call := n.calls[req.Method]
	n.calls[req.Method]++
	n.mu.Unlock()

	response := map[string]any{"jsonrpc": "2.0", "id": req.ID}
	switch {
	case len(script) == 0:
		response["error"] = map[string]any{"code": -32601, "message": "method not found: " + req.Method}
	case script[min(call, len(script)-1)].drop:
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}
		return
	case script[min(call, len(script)-1)].nodeErr != "":
		response["error"] = map[string]any{"code": -32000, "message": script[min(call, len(script)-1)].nodeErr}
	default:
		response["result"] = script[min(call, len(script)-1)].result
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func (n *fakeNode) script(method string, replies ...reply) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.scripts[method] = replies
}

func (n *fakeNode) count(method string) int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.calls[method]
}

// newFakeNode serves a healthy chain whose scripts overrides replace.
func newFakeNode(t *testing.T, overrides map[string][]reply) (*fakeNode, *EVMClient) {
	t.Helper()
	header, err := json.Marshal(&types.Header{Number: big.NewInt(1), Difficulty: big.NewInt(0), BaseFee: big.NewInt(7)})
	require.NoError(t, err)
	node := &fakeNode{
		scripts: map[string][]reply{
			"eth_chainId":               {{result: "0x1"}},
			"eth_getTransactionCount":   {{result: "0x0"}},
			"eth_estimateGas":           {{result: "0x5208"}},
			"eth_maxPriorityFeePerGas":  {{result: "0x1"}},
			"eth_getBlockByNumber":      {{result: json.RawMessage(header)}},
			"eth_sendRawTransaction":    {{result: common.HexToHash("0x01")}},
			"eth_getTransactionReceipt": {receiptReply(t)},
		},
		calls: map[string]int{},
	}
	for method, script := range overrides {
		node.scripts[method] = script
	}
	server := httptest.NewServer(node)
	t.Cleanup(server.Close)
	ec, err := Dial(t.Context(), server.URL)
	require.NoError(t, err)
	client, err := NewConnectedClient(t.Context(), ec)
	require.NoError(t, err)
	t.Cleanup(client.Close)
	return node, client
}

var testWait = TransactionWait{Timeout: 2 * time.Second, PollInterval: 10 * time.Millisecond}

func broadcast(t *testing.T, client *EVMClient) error {
	t.Helper()
	from, err := NewAccount()
	require.NoError(t, err)
	to := common.HexToAddress("0x02")
	_, err = client.BroadcastTx(t.Context(), testWait, from, &to, nil, nil)
	return err
}

func TestBroadcastTxRetriesAMissingLatestHeader(t *testing.T) {
	header, err := json.Marshal(&types.Header{Number: big.NewInt(1), Difficulty: big.NewInt(0)})
	require.NoError(t, err)
	node, client := newFakeNode(t, map[string][]reply{
		"eth_getBlockByNumber": {{result: nil}, {result: json.RawMessage(header)}},
	})

	require.NoError(t, broadcast(t, client))
	assert.Equal(t, 2, node.count("eth_getBlockByNumber"))
}

func TestBroadcastTxRetriesUnansweredRequests(t *testing.T) {
	node, client := newFakeNode(t, map[string][]reply{
		"eth_getTransactionCount":   {{drop: true}, {result: "0x0"}},
		"eth_estimateGas":           {{drop: true}, {result: "0x5208"}},
		"eth_getTransactionReceipt": {{drop: true}, {nodeErr: "busy"}, {result: nil}, receiptReply(t)},
	})

	require.NoError(t, broadcast(t, client))
	assert.Equal(t, 4, node.count("eth_getTransactionReceipt"), "a failed poll is retried at the next")
}

func TestBroadcastTxLeavesARefusedResendToTheReceipt(t *testing.T) {
	node, client := newFakeNode(t, map[string][]reply{
		"eth_sendRawTransaction": {{drop: true}, {nodeErr: "already known"}},
	})

	require.NoError(t, broadcast(t, client), "the unanswered first send may have arrived")
	assert.Equal(t, 2, node.count("eth_sendRawTransaction"))
	assert.Positive(t, node.count("eth_getTransactionReceipt"))
}

func TestBroadcastTxFailsWhenTheNodeRefuses(t *testing.T) {
	node, client := newFakeNode(t, map[string][]reply{
		"eth_sendRawTransaction": {{nodeErr: "insufficient funds"}},
		"eth_estimateGas":        {{nodeErr: "execution reverted"}},
	})

	require.ErrorContains(t, broadcast(t, client), "execution reverted")
	assert.Equal(t, 2, node.count("eth_estimateGas"), "a node error is not retried, beyond Besu's fallback")
	assert.Zero(t, node.count("eth_sendRawTransaction"))

	node.script("eth_estimateGas", reply{result: "0x5208"})
	require.ErrorContains(t, broadcast(t, client), "insufficient funds")
	assert.Equal(t, 1, node.count("eth_sendRawTransaction"))
	assert.Zero(t, node.count("eth_getTransactionReceipt"))
}

func TestBroadcastTxSharesAFeeReading(t *testing.T) {
	node, client := newFakeNode(t, nil)

	require.NoError(t, broadcast(t, client))
	require.NoError(t, broadcast(t, client))
	assert.Equal(t, 1, node.count("eth_maxPriorityFeePerGas"))
	assert.Equal(t, 1, node.count("eth_getBlockByNumber"))
}

// receiptReply answers with a successful receipt.
func receiptReply(t *testing.T) reply {
	t.Helper()
	receipt, err := json.Marshal(&types.Receipt{
		Status: types.ReceiptStatusSuccessful, TxHash: common.HexToHash("0x01"), Logs: []*types.Log{},
	})
	require.NoError(t, err)
	return reply{result: json.RawMessage(receipt)}
}
