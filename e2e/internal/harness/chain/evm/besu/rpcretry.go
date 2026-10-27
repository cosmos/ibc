// SPDX-License-Identifier: Apache-2.0

package besu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

const (
	// readRetryAttempts are spread over one block period, the window in which
	// an import can race a read.
	readRetryAttempts = 8

	// Besu answers reads with a bare Internal error, without revert data,
	// when they race the import of the next block.
	jsonRPCInternalError = -32603
)

// readMethods are the JSON-RPC methods the harness reads chain state with.
// Only these are retried; anything that writes is sent once.
var readMethods = map[string]bool{
	"eth_blockNumber":         true,
	"eth_call":                true,
	"eth_chainId":             true,
	"eth_estimateGas":         true,
	"eth_getBalance":          true,
	"eth_getBlockByHash":      true,
	"eth_getBlockByNumber":    true,
	"eth_getCode":             true,
	"eth_getLogs":             true,
	"eth_getProof":            true,
	"eth_getStorageAt":        true,
	"eth_getTransactionCount": true,
}

// dialClient dials a Besu node's HTTP RPC through readRetryTransport.
func dialClient(ctx context.Context, url string, blockPeriod time.Duration) (*ethclient.Client, error) {
	client, err := rpc.DialOptions(ctx, url, rpc.WithHTTPClient(&http.Client{
		Transport: readRetryTransport{base: http.DefaultTransport, delay: blockPeriod / readRetryAttempts},
	}))
	if err != nil {
		return nil, err
	}
	return ethclient.NewClient(client), nil
}

// readRetryTransport retries reads that Besu fails while it imports a block:
// a JSON-RPC Internal error, or no block for "latest". Tests read state
// between blocks and are not checking RPC reliability, so a read that
// succeeds a moment later must not fail them.
type readRetryTransport struct {
	base  http.RoundTripper
	delay time.Duration
}

func (t readRetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body == nil {
		return t.base.RoundTrip(req)
	}
	body, err := io.ReadAll(req.Body)
	_ = req.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("besu rpc: read request: %w", err)
	}
	var call struct {
		Method string            `json:"method"`
		Params []json.RawMessage `json:"params"`
	}
	// Batches fail to decode into a single call and are sent once.
	if json.Unmarshal(body, &call) != nil || !readMethods[call.Method] {
		return t.base.RoundTrip(withBody(req, body))
	}

	for attempt := 1; ; attempt++ {
		resp, err := t.base.RoundTrip(withBody(req, body))
		if err != nil || resp.StatusCode != http.StatusOK || attempt == readRetryAttempts {
			return resp, err
		}
		respBody, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("besu rpc: read %s response: %w", call.Method, err)
		}
		resp.Body = io.NopCloser(bytes.NewReader(respBody))
		if !transientRead(call.Method, call.Params, respBody) {
			return resp, nil
		}
		select {
		case <-req.Context().Done():
			return nil, fmt.Errorf("besu rpc: %s retry after transient reply: %w", call.Method, req.Context().Err())
		case <-time.After(t.delay):
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
	// The latest block always exists, so null means Besu has not finished
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
