// SPDX-License-Identifier: Apache-2.0

package besu

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
)

const (
	internalErrorReply = `{"jsonrpc":"2.0","id":%s,"error":{"code":-32603,"message":"Internal error"}}`
	nullReply          = `{"jsonrpc":"2.0","id":%s,"result":null}`
	revertReply        = `{"jsonrpc":"2.0","id":%s,"error":{"code":3,"message":"execution reverted","data":"0x"}}`
)

// scriptedNode answers each call to a method with that method's next scripted
// reply, repeating the last one, and counts the calls.
type scriptedNode struct {
	mu      sync.Mutex
	replies map[string][]string
	calls   map[string]int
}

func (n *scriptedNode) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var call struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
	}
	if err := json.Unmarshal(body, &call); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	n.mu.Lock()
	replies := n.replies[call.Method]
	reply := replies[min(n.calls[call.Method], len(replies)-1)]
	n.calls[call.Method]++
	n.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, reply, call.ID)
}

func (n *scriptedNode) callCount(method string) int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.calls[method]
}

func dialScripted(
	t *testing.T,
	delay time.Duration,
	replies map[string][]string,
) (*ethclient.Client, *scriptedNode) {
	t.Helper()
	node := &scriptedNode{replies: replies, calls: map[string]int{}}
	server := httptest.NewServer(node)
	t.Cleanup(server.Close)
	client, err := rpc.DialOptions(t.Context(), server.URL, rpc.WithHTTPClient(&http.Client{
		Transport: readRetryTransport{base: http.DefaultTransport, delay: delay},
	}))
	require.NoError(t, err)
	t.Cleanup(client.Close)
	return ethclient.NewClient(client), node
}

func TestReadRetryTransportRetriesInternalError(t *testing.T) {
	client, node := dialScripted(t, time.Millisecond, map[string][]string{
		"eth_call": {internalErrorReply, internalErrorReply, `{"jsonrpc":"2.0","id":%s,"result":"0x2a"}`},
	})
	to := common.HexToAddress("0x1")

	got, err := client.CallContract(t.Context(), ethereum.CallMsg{To: &to}, nil)
	require.NoError(t, err)
	require.Equal(t, []byte{0x2a}, got)
	require.Equal(t, 3, node.callCount("eth_call"))
}

func TestReadRetryTransportRetriesMissingLatestBlock(t *testing.T) {
	header, err := json.Marshal(&types.Header{Number: big.NewInt(7), Difficulty: big.NewInt(0)})
	require.NoError(t, err)
	client, node := dialScripted(t, time.Millisecond, map[string][]string{
		"eth_getBlockByNumber": {nullReply, `{"jsonrpc":"2.0","id":%s,"result":` + string(header) + `}`},
	})

	got, err := client.HeaderByNumber(t.Context(), nil)
	require.NoError(t, err)
	require.Equal(t, big.NewInt(7), got.Number)
	require.Equal(t, 2, node.callCount("eth_getBlockByNumber"))
}

func TestReadRetryTransportReturnsLastErrorAfterAttempts(t *testing.T) {
	client, node := dialScripted(t, time.Millisecond, map[string][]string{"eth_call": {internalErrorReply}})
	to := common.HexToAddress("0x1")

	_, err := client.CallContract(t.Context(), ethereum.CallMsg{To: &to}, nil)
	require.ErrorContains(t, err, "Internal error")
	require.Equal(t, readRetryAttempts, node.callCount("eth_call"))
}

func TestReadRetryTransportReturnsContextErrorWhenCanceledDuringDelay(t *testing.T) {
	client, node := dialScripted(t, time.Minute, map[string][]string{"eth_call": {internalErrorReply}})
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	to := common.HexToAddress("0x1")

	_, err := client.CallContract(ctx, ethereum.CallMsg{To: &to}, nil)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, 1, node.callCount("eth_call"))
}

func TestReadRetryTransportSendsOnceWhenNotTransient(t *testing.T) {
	client, node := dialScripted(t, time.Millisecond, map[string][]string{
		"eth_call":                  {revertReply},
		"eth_getBlockByNumber":      {nullReply},
		"eth_getTransactionReceipt": {nullReply},
		"eth_sendRawTransaction":    {internalErrorReply},
	})
	ctx := t.Context()
	to := common.HexToAddress("0x1")

	_, err := client.CallContract(ctx, ethereum.CallMsg{To: &to}, nil)
	require.ErrorContains(t, err, "execution reverted")
	_, err = client.HeaderByNumber(ctx, big.NewInt(9))
	require.ErrorIs(t, err, ethereum.NotFound)
	_, err = client.TransactionReceipt(ctx, common.Hash{})
	require.ErrorIs(t, err, ethereum.NotFound)
	tx := types.NewTx(&types.LegacyTx{Gas: 21_000, GasPrice: big.NewInt(0), To: &to})
	require.ErrorContains(t, client.SendTransaction(ctx, tx), "Internal error")

	for _, method := range []string{
		"eth_call", "eth_getBlockByNumber", "eth_getTransactionReceipt", "eth_sendRawTransaction",
	} {
		require.Equal(t, 1, node.callCount(method), method)
	}
}
