// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"go.opentelemetry.io/otel/metric"

	"github.com/cosmos/ibc/cli/internal/otel"
)

type instrumentation struct {
	Operation metric.Float64Histogram
}

var metrics = otel.RegisterMetrics("evm_client", newInstrumentation)

func newInstrumentation(m metric.Meter) (*instrumentation, error) {
	operation, err := m.Float64Histogram("evm_operation_dur", otel.UnitMilliseconds())
	if err != nil {
		return nil, err
	}

	return &instrumentation{Operation: operation}, nil
}

func meteredEthClient(ctx context.Context, chainID, rpcURL string) (*ethclient.Client, error) {
	httpClient := &http.Client{
		Transport: newMetricsTransport(chainID, http.DefaultTransport),
	}

	rpcClient, err := rpc.DialOptions(ctx, rpcURL, rpc.WithHTTPClient(httpClient))
	if err != nil {
		return nil, err
	}

	return ethclient.NewClient(rpcClient), nil
}

type metricsTransport struct {
	base    http.RoundTripper
	chainID string
	metrics *instrumentation
}

func newMetricsTransport(chainID string, base http.RoundTripper) *metricsTransport {
	if base == nil {
		base = http.DefaultTransport
	}

	return &metricsTransport{base, chainID, metrics}
}

func (t *metricsTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	operation, ok := t.extractMethod(req)
	if !ok {
		return t.base.RoundTrip(req)
	}

	started := time.Now()
	resp, err := t.base.RoundTrip(req)

	var resultCode int
	if resp != nil {
		resultCode = resp.StatusCode
	} else {
		resultCode = -1
	}

	otel.RecordOperation(
		req.Context(),
		t.metrics.Operation,
		operation,
		started,
		otel.AttrChainID.String(t.chainID),
		otel.AttrResult.Int(resultCode),
	)

	return resp, err
}

func (t *metricsTransport) extractMethod(req *http.Request) (string, bool) {
	if req == nil || req.Body == nil {
		return "", false
	}

	body, err := readRequestBody(req)
	if err != nil {
		return "", false
	}

	type rpcShape struct {
		JSONRPC string `json:"jsonrpc"`
		Method  string `json:"method"`
	}

	var rpcReq rpcShape
	err = json.Unmarshal(body, &rpcReq)

	if err != nil || rpcReq.JSONRPC != "2.0" || rpcReq.Method == "" {
		return "", false
	}

	return rpcReq.Method, true
}

// double body write, but since evm payloads are small, it's fine
func readRequestBody(req *http.Request) ([]byte, error) {
	body, err := io.ReadAll(req.Body)

	// place bytes back
	req.Body = io.NopCloser(bytes.NewReader(body))

	return body, err
}
