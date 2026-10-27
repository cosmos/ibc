// SPDX-License-Identifier: Apache-2.0

// Package evmrpc dials EVM JSON-RPC nodes, retrying reads that a node fails
// transiently.
package evmrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/rpc"
)

const (
	// DefaultAttempts is how many times RetryTransport sends a read.
	DefaultAttempts = 8
	// DefaultDelay is how long RetryTransport waits between attempts.
	DefaultDelay = 250 * time.Millisecond

	// Nodes such as Besu and Anvil answer a read with a bare Internal error,
	// without revert data, when it fails transiently.
	jsonRPCInternalError = -32603

	// Callers sign transactions locally, so this is the only write they send.
	// A resend after the node accepted the transaction would fail as already
	// known, so it is sent once.
	sendRawTransaction = "eth_sendRawTransaction"
)

// Dial dials url with HTTP requests sent through t. Websocket and IPC URLs
// bypass t.
func Dial(ctx context.Context, url string, t RetryTransport) (*rpc.Client, error) {
	return rpc.DialOptions(ctx, url, rpc.WithHTTPClient(&http.Client{Transport: t}))
}

// RetryTransport retries reads that a node fails transiently: a JSON-RPC
// Internal error, or no block for "latest". Writes and batches are sent once.
// Zero fields take their defaults: http.DefaultTransport, DefaultDelay and
// DefaultAttempts.
type RetryTransport struct {
	Base     http.RoundTripper
	Delay    time.Duration
	Attempts int
}

// RoundTrip implements http.RoundTripper.
func (t RetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base, delay, attempts := t.Base, t.Delay, t.Attempts
	if base == nil {
		base = http.DefaultTransport
	}
	if delay <= 0 {
		delay = DefaultDelay
	}
	if attempts <= 0 {
		attempts = DefaultAttempts
	}

	if req.Body == nil {
		return base.RoundTrip(req)
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
	if json.Unmarshal(body, &call) != nil || call.Method == sendRawTransaction {
		return base.RoundTrip(withBody(req, body))
	}

	for attempt := 1; ; attempt++ {
		resp, err := base.RoundTrip(withBody(req, body))
		if err != nil || resp.StatusCode != http.StatusOK || attempt == attempts {
			return resp, err
		}
		respBody, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("evm rpc: read %s response: %w", call.Method, err)
		}
		resp.Body = io.NopCloser(bytes.NewReader(respBody))
		if !transientRead(call.Method, call.Params, respBody) {
			return resp, nil
		}
		select {
		case <-req.Context().Done():
			return nil, fmt.Errorf("evm rpc: %s retry after transient reply: %w", call.Method, req.Context().Err())
		case <-time.After(delay):
		}
	}
}

func transientRead(method string, params []json.RawMessage, respBody []byte) bool {
	var reply struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(respBody, &reply) != nil {
		return false
	}
	if reply.Error != nil {
		return reply.Error.Code == jsonRPCInternalError
	}
	// The latest block always exists, so null means the node has not finished
	// importing it.
	return method == "eth_getBlockByNumber" && len(params) > 0 &&
		string(params[0]) == `"latest"` && string(reply.Result) == "null"
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
