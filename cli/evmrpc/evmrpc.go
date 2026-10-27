// SPDX-License-Identifier: Apache-2.0

// Package evmrpc dials EVM JSON-RPC nodes, retrying standard reads that a node
// fails transiently. Metrics above the client see each call's final outcome
// and its latency including retries.
package evmrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

const (
	attempts = 8
	delay    = 250 * time.Millisecond

	// Anvil and Besu answer transiently failed reads with an Internal error.
	jsonRPCInternalError = -32603
)

// pooledTransport keeps connections to each node open for reuse. go-ethereum's
// default HTTP client keeps two idle per host, so concurrent calls beyond two
// each open and close a connection, and under load the closed connections
// hold every local port in TIME_WAIT.
var pooledTransport = func() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 0
	transport.MaxIdleConnsPerHost = 256
	return transport
}()

// Dial dials url. Over HTTP, reads are retried; websocket and IPC URLs are not.
func Dial(ctx context.Context, url string) (*ethclient.Client, error) {
	return dial(ctx, url, retryTransport{base: pooledTransport, delay: delay})
}

func dial(ctx context.Context, url string, transport http.RoundTripper) (*ethclient.Client, error) {
	client, err := rpc.DialOptions(ctx, url, rpc.WithHTTPClient(&http.Client{Transport: transport}))
	if err != nil {
		return nil, err
	}
	return ethclient.NewClient(client), nil
}

// retryTransport retries standard reads, eth_ methods other than eth_send*,
// that a node answers with an Internal error or with no latest block. Writes,
// vendor methods such as anvil_mine that a retry could apply twice, and
// batches are sent once.
type retryTransport struct {
	base  http.RoundTripper
	delay time.Duration
}

func (t retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body == nil {
		return t.base.RoundTrip(req)
	}
	body, err := io.ReadAll(req.Body)
	_ = req.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("evm rpc: read request: %w", err)
	}
	var call struct {
		Method string            `json:"method"`
		Params []json.RawMessage `json:"params"`
	}
	// Batches fail to decode into a single call and are sent once.
	if json.Unmarshal(body, &call) != nil || !isRead(call.Method) {
		return t.base.RoundTrip(withBody(req, body))
	}
	latestBlock := call.Method == "eth_getBlockByNumber" && len(call.Params) > 0 &&
		string(call.Params[0]) == `"latest"`

	for attempt := 1; ; attempt++ {
		resp, err := t.base.RoundTrip(withBody(req, body))
		if err != nil || resp.StatusCode != http.StatusOK || attempt == attempts {
			return resp, err
		}
		respBody, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("evm rpc: read %s response: %w", call.Method, err)
		}
		resp.Body = io.NopCloser(bytes.NewReader(respBody))
		if !transientRead(respBody, latestBlock) {
			return resp, nil
		}
		select {
		case <-req.Context().Done():
			return nil, fmt.Errorf("evm rpc: %s retry after transient reply: %w", call.Method, req.Context().Err())
		case <-time.After(t.delay):
		}
	}
}

func isRead(method string) bool {
	return strings.HasPrefix(method, "eth_") && !strings.HasPrefix(method, "eth_send")
}

func transientRead(respBody []byte, latestBlock bool) bool {
	var reply struct {
		Error *struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(respBody, &reply) != nil {
		return false
	}
	if reply.Error != nil {
		return reply.Error.Code == jsonRPCInternalError
	}
	if !latestBlock {
		return false
	}
	// The latest block always exists, so null means the node has not finished
	// importing it. Only this reply's result is decoded.
	var block struct {
		Result json.RawMessage `json:"result"`
	}
	return json.Unmarshal(respBody, &block) == nil && string(block.Result) == "null"
}

func withBody(req *http.Request, body []byte) *http.Request {
	clone := req.Clone(req.Context())
	clone.Body = io.NopCloser(bytes.NewReader(body))
	clone.ContentLength = int64(len(body))
	clone.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	return clone
}
