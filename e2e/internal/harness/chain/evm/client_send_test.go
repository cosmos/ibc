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

// reply is one scripted answer: a result, a node error, an HTTP error
// status, or a dropped connection.
type reply struct {
	result  any
	nodeErr string
	status  int
	drop    bool
}

// fakeNode answers JSON-RPC requests from per-method scripts; a script's last
// reply repeats.
type fakeNode struct {
	mu      sync.Mutex
	scripts map[string][]reply
	calls   map[string]int
	// sentNonces are the nonces of the raw transactions sent, in order.
	sentNonces []uint64
}

func (n *fakeNode) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID     json.RawMessage   `json:"id"`
		Method string            `json:"method"`
		Params []json.RawMessage `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	n.mu.Lock()
	if req.Method == "eth_sendRawTransaction" && len(req.Params) == 1 {
		var raw string
		var tx types.Transaction
		if json.Unmarshal(req.Params[0], &raw) == nil && tx.UnmarshalBinary(common.FromHex(raw)) == nil {
			n.sentNonces = append(n.sentNonces, tx.Nonce())
		}
	}
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
	case script[min(call, len(script)-1)].status != 0:
		w.WriteHeader(script[min(call, len(script)-1)].status)
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
	return broadcastFrom(t, client, from)
}

func broadcastFrom(t *testing.T, client *EVMClient, from Account) error {
	t.Helper()
	to := common.HexToAddress("0x02")
	_, err := client.BroadcastTx(t.Context(), testWait, from, &to, nil, nil)
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
		"eth_sendRawTransaction":   {{drop: true}, {nodeErr: "already known"}},
		"eth_getTransactionByHash": {knownTxReply(t)},
	})

	require.NoError(t, broadcast(t, client), "the unanswered first send arrived")
	assert.Equal(t, 2, node.count("eth_sendRawTransaction"))
	assert.Positive(t, node.count("eth_getTransactionReceipt"))
}

func TestBroadcastTxReportsARefusalOfATransactionTheNodeLacks(t *testing.T) {
	node, client := newFakeNode(t, map[string][]reply{
		"eth_sendRawTransaction":   {{drop: true}, {nodeErr: "insufficient funds"}},
		"eth_getTransactionByHash": {{result: nil}},
	})

	require.ErrorContains(t, broadcast(t, client), "insufficient funds")
	assert.Equal(t, 1, node.count("eth_getTransactionByHash"))
	assert.Zero(t, node.count("eth_getTransactionReceipt"))
}

func TestBroadcastTxRetriesOnlyTheHTTPStatusesThatAskTo(t *testing.T) {
	node, client := newFakeNode(t, map[string][]reply{
		"eth_estimateGas": {
			{status: http.StatusServiceUnavailable},
			{status: http.StatusTooManyRequests},
			{result: "0x5208"},
		},
	})
	require.NoError(t, broadcast(t, client))
	assert.Equal(t, 3, node.count("eth_estimateGas"))

	node.script("eth_estimateGas", reply{status: http.StatusNotFound})
	require.ErrorContains(t, broadcast(t, client), "404")
	assert.Equal(t, 5, node.count("eth_estimateGas"), "one call and Besu's fallback")
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

// receiptReply answers with a successful receipt.
func receiptReply(t *testing.T) reply {
	t.Helper()
	receipt, err := json.Marshal(&types.Receipt{
		Status: types.ReceiptStatusSuccessful, TxHash: common.HexToHash("0x01"), Logs: []*types.Log{},
	})
	require.NoError(t, err)
	return reply{result: json.RawMessage(receipt)}
}

// knownTxReply answers with a signed transaction, as a node that holds it.
func knownTxReply(t *testing.T) reply {
	t.Helper()
	from, err := NewAccount()
	require.NoError(t, err)
	tx, err := types.SignNewTx(from.key, types.LatestSignerForChainID(big.NewInt(1)), &types.DynamicFeeTx{
		ChainID: big.NewInt(1), Gas: 21_000, GasFeeCap: big.NewInt(1), GasTipCap: big.NewInt(1),
	})
	require.NoError(t, err)
	encoded, err := json.Marshal(tx)
	require.NoError(t, err)
	return reply{result: json.RawMessage(encoded)}
}

func TestBroadcastTxNeverReusesAnIncludedNonce(t *testing.T) {
	// The node keeps reporting nonce 0, as a stale pending read would.
	reverted, err := json.Marshal(&types.Receipt{
		Status: types.ReceiptStatusFailed, TxHash: common.HexToHash("0x01"), Logs: []*types.Log{},
	})
	require.NoError(t, err)
	node, client := newFakeNode(t, map[string][]reply{
		"eth_getTransactionReceipt": {receiptReply(t), {result: json.RawMessage(reverted)}, receiptReply(t)},
	})
	from, err := NewAccount()
	require.NoError(t, err)

	require.NoError(t, broadcastFrom(t, client, from))
	require.ErrorContains(t, broadcastFrom(t, client, from), "reverted")
	require.NoError(t, broadcastFrom(t, client, from))
	require.NoError(t, broadcast(t, client), "another account has its own floor")

	node.mu.Lock()
	defer node.mu.Unlock()
	assert.Equal(t, []uint64{0, 1, 2, 0}, node.sentNonces, "a reverted transaction spends its nonce too")
}

func TestBroadcastTxRefusalReportsTheNonce(t *testing.T) {
	_, client := newFakeNode(t, map[string][]reply{
		"eth_sendRawTransaction":  {{nodeErr: "Nonce too low"}},
		"eth_getTransactionCount": {{result: "0x3"}},
	})

	err := broadcast(t, client)

	require.ErrorContains(t, err, "nonce 3 (nonce floor 0)")
	require.ErrorContains(t, err, "Nonce too low")
}
