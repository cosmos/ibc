// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/cosmos/ibc/cli/internal/tests/mocks"
)

type jsonRPCError struct{ code int }

func (e jsonRPCError) Error() string  { return fmt.Sprintf("jsonrpc %d", e.code) }
func (e jsonRPCError) ErrorCode() int { return e.code }

func TestMeteredClient(t *testing.T) {
	t.Run("recordsResultAndCode", func(t *testing.T) {
		for _, tt := range []struct {
			name   string
			err    error
			result string
			code   string
		}{
			{name: "ok", result: "ok"},
			{name: "notFoundIsAnAnswer", err: ethereum.NotFound, result: "ok"},
			{name: "httpStatus", err: rpc.HTTPError{StatusCode: http.StatusTooManyRequests}, result: "error", code: "http_429"},
			{name: "wrappedHTTPStatus", err: fmt.Errorf("x: %w", rpc.HTTPError{StatusCode: 500}), result: "error", code: "http_500"},
			{name: "jsonRPCCode", err: jsonRPCError{code: -32000}, result: "error", code: "jsonrpc_-32000"},
			{name: "executionReverted", err: jsonRPCError{code: 3}, result: "error", code: "jsonrpc_3"},
			{name: "canceled", err: context.Canceled, result: "error", code: "other"},
			{name: "transport", err: errors.New("connection refused"), result: "error", code: "other"},
		} {
			t.Run(tt.name, func(t *testing.T) {
				// ARRANGE
				ctx := context.Background()
				client, eth, reader := newTestMeteredClient(t)
				eth.EXPECT().HeaderByNumber(ctx, (*big.Int)(nil)).Return(&types.Header{}, tt.err).Once()

				// ACT
				_, err := client.HeaderByNumber(ctx, nil)

				// ASSERT
				if tt.err == nil {
					require.NoError(t, err)
				} else {
					require.EqualError(t, err, tt.err.Error())
				}
				point := requireSingleOperation(t, reader)
				assert.ElementsMatch(t, []attribute.KeyValue{
					attribute.String("operation", "eth_getBlockByNumber"),
					attribute.String("chain_id", chainIDEth),
					attribute.String("result", tt.result),
					attribute.String("code", tt.code),
				}, point.Attributes.ToSlice())
			})
		}
	})

	// every ETHClient method is instrumented and labeled with the JSON-RPC
	// method it issues
	t.Run("labelsEveryMethod", func(t *testing.T) {
		ctx := context.Background()
		address := common.HexToAddress("0x01")
		hash := common.HexToHash("0x02")

		for _, tt := range []struct {
			operation string
			expect    func(eth *mocks.MockETHClient)
			call      func(client ETHClient) error
		}{
			{
				operation: "eth_getCode",
				expect: func(eth *mocks.MockETHClient) {
					eth.EXPECT().CodeAt(ctx, address, (*big.Int)(nil)).Return(nil, assert.AnError).Once()
				},
				call: func(c ETHClient) error { _, err := c.CodeAt(ctx, address, nil); return err },
			},
			{
				operation: "eth_call",
				expect: func(eth *mocks.MockETHClient) {
					eth.EXPECT().CallContract(ctx, mock.Anything, (*big.Int)(nil)).Return(nil, assert.AnError).Once()
				},
				call: func(c ETHClient) error { _, err := c.CallContract(ctx, ethereum.CallMsg{}, nil); return err },
			},
			{
				operation: "eth_getBlockByNumber",
				expect: func(eth *mocks.MockETHClient) {
					eth.EXPECT().HeaderByNumber(ctx, (*big.Int)(nil)).Return(nil, assert.AnError).Once()
				},
				call: func(c ETHClient) error { _, err := c.HeaderByNumber(ctx, nil); return err },
			},
			{
				operation: "eth_getCode",
				expect: func(eth *mocks.MockETHClient) {
					eth.EXPECT().PendingCodeAt(ctx, address).Return(nil, assert.AnError).Once()
				},
				call: func(c ETHClient) error { _, err := c.PendingCodeAt(ctx, address); return err },
			},
			{
				operation: "eth_getTransactionCount",
				expect: func(eth *mocks.MockETHClient) {
					eth.EXPECT().PendingNonceAt(ctx, address).Return(0, assert.AnError).Once()
				},
				call: func(c ETHClient) error { _, err := c.PendingNonceAt(ctx, address); return err },
			},
			{
				operation: "eth_gasPrice",
				expect: func(eth *mocks.MockETHClient) {
					eth.EXPECT().SuggestGasPrice(ctx).Return(nil, assert.AnError).Once()
				},
				call: func(c ETHClient) error { _, err := c.SuggestGasPrice(ctx); return err },
			},
			{
				operation: "eth_maxPriorityFeePerGas",
				expect: func(eth *mocks.MockETHClient) {
					eth.EXPECT().SuggestGasTipCap(ctx).Return(nil, assert.AnError).Once()
				},
				call: func(c ETHClient) error { _, err := c.SuggestGasTipCap(ctx); return err },
			},
			{
				operation: "eth_estimateGas",
				expect: func(eth *mocks.MockETHClient) {
					eth.EXPECT().EstimateGas(ctx, mock.Anything).Return(0, assert.AnError).Once()
				},
				call: func(c ETHClient) error { _, err := c.EstimateGas(ctx, ethereum.CallMsg{}); return err },
			},
			{
				operation: "eth_sendRawTransaction",
				expect: func(eth *mocks.MockETHClient) {
					eth.EXPECT().SendTransaction(ctx, mock.Anything).Return(assert.AnError).Once()
				},
				call: func(c ETHClient) error { return c.SendTransaction(ctx, types.NewTx(&types.LegacyTx{})) },
			},
			{
				operation: "eth_getLogs",
				expect: func(eth *mocks.MockETHClient) {
					eth.EXPECT().FilterLogs(ctx, mock.Anything).Return(nil, assert.AnError).Once()
				},
				call: func(c ETHClient) error { _, err := c.FilterLogs(ctx, ethereum.FilterQuery{}); return err },
			},
			{
				operation: "eth_subscribe",
				expect: func(eth *mocks.MockETHClient) {
					eth.EXPECT().SubscribeFilterLogs(ctx, mock.Anything, mock.Anything).Return(nil, assert.AnError).Once()
				},
				call: func(c ETHClient) error {
					_, err := c.SubscribeFilterLogs(ctx, ethereum.FilterQuery{}, make(chan types.Log))
					return err
				},
			},
			{
				operation: "eth_getBalance",
				expect: func(eth *mocks.MockETHClient) {
					eth.EXPECT().BalanceAt(ctx, address, (*big.Int)(nil)).Return(nil, assert.AnError).Once()
				},
				call: func(c ETHClient) error { _, err := c.BalanceAt(ctx, address, nil); return err },
			},
			{
				operation: "eth_getTransactionReceipt",
				expect: func(eth *mocks.MockETHClient) {
					eth.EXPECT().TransactionReceipt(ctx, hash).Return(nil, assert.AnError).Once()
				},
				call: func(c ETHClient) error { _, err := c.TransactionReceipt(ctx, hash); return err },
			},
			{
				operation: "eth_getTransactionByHash",
				expect: func(eth *mocks.MockETHClient) {
					eth.EXPECT().TransactionByHash(ctx, hash).Return(nil, false, assert.AnError).Once()
				},
				call: func(c ETHClient) error { _, _, err := c.TransactionByHash(ctx, hash); return err },
			},
			{
				operation: "eth_getStorageAt",
				expect: func(eth *mocks.MockETHClient) {
					eth.EXPECT().StorageAt(ctx, address, hash, (*big.Int)(nil)).Return(nil, assert.AnError).Once()
				},
				call: func(c ETHClient) error { _, err := c.StorageAt(ctx, address, hash, nil); return err },
			},
		} {
			t.Run(tt.operation, func(t *testing.T) {
				// ARRANGE
				client, eth, reader := newTestMeteredClient(t)
				tt.expect(eth)

				// ACT
				err := tt.call(client)

				// ASSERT
				require.ErrorIs(t, err, assert.AnError)
				point := requireSingleOperation(t, reader)
				operation, _ := point.Attributes.Value("operation")
				assert.Equal(t, tt.operation, operation.AsString())
			})
		}
	})
}

func newTestMeteredClient(t *testing.T) (*meteredClient, *mocks.MockETHClient, *sdkmetric.ManualReader) {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() {
		require.NoError(t, provider.Shutdown(context.Background()))
	})

	instruments, err := newInstrumentation(provider.Meter("test"))
	require.NoError(t, err)

	eth := mocks.NewMockETHClient(t)

	return &meteredClient{eth: eth, chainID: chainIDEth, metrics: instruments}, eth, reader
}

func requireSingleOperation(t *testing.T, reader *sdkmetric.ManualReader) metricdata.HistogramDataPoint[float64] {
	t.Helper()

	var data metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &data))
	require.Len(t, data.ScopeMetrics, 1)
	require.Len(t, data.ScopeMetrics[0].Metrics, 1)
	require.Equal(t, "evm_operation_dur", data.ScopeMetrics[0].Metrics[0].Name)

	histogram, ok := data.ScopeMetrics[0].Metrics[0].Data.(metricdata.Histogram[float64])
	require.True(t, ok)
	require.Len(t, histogram.DataPoints, 1)
	assert.Equal(t, uint64(1), histogram.DataPoints[0].Count)

	return histogram.DataPoints[0]
}

// New wraps the dialed client, and the labels hold for what go-ethereum
// really returns rather than for hand-built errors.
func TestNewRecordsRealResponses(t *testing.T) {
	ctx := context.Background()
	hash := common.HexToHash("0x02")

	for _, tt := range []struct {
		name      string
		status    int
		body      string // {{id}} is replaced with the request id
		call      func(ETHClient) error
		operation string
		result    string
		code      string
	}{
		{
			name:      "ok",
			status:    http.StatusOK,
			body:      `{"jsonrpc":"2.0","id":{{id}},"result":"0x7"}`,
			call:      func(c ETHClient) error { _, err := c.PendingNonceAt(ctx, common.Address{}); return err },
			operation: "eth_getTransactionCount",
			result:    "ok",
		},
		{
			name:      "nullResultIsAnAnswer",
			status:    http.StatusOK,
			body:      `{"jsonrpc":"2.0","id":{{id}},"result":null}`,
			call:      func(c ETHClient) error { _, err := c.TransactionReceipt(ctx, hash); return err },
			operation: "eth_getTransactionReceipt",
			result:    "ok",
		},
		{
			name:      "jsonRPCError",
			status:    http.StatusOK,
			body:      `{"jsonrpc":"2.0","id":{{id}},"error":{"code":-32000,"message":"header not found"}}`,
			call:      func(c ETHClient) error { _, err := c.PendingNonceAt(ctx, common.Address{}); return err },
			operation: "eth_getTransactionCount",
			result:    "error",
			code:      "jsonrpc_-32000",
		},
		{
			name:      "httpStatus",
			status:    http.StatusTooManyRequests,
			body:      `rate limited`,
			call:      func(c ETHClient) error { _, err := c.PendingNonceAt(ctx, common.Address{}); return err },
			operation: "eth_getTransactionCount",
			result:    "error",
			code:      "http_429",
		},
		{
			name:      "malformedBody",
			status:    http.StatusOK,
			body:      `<html>bad gateway</html>`,
			call:      func(c ETHClient) error { _, err := c.PendingNonceAt(ctx, common.Address{}); return err },
			operation: "eth_getTransactionCount",
			result:    "error",
			code:      "other",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			reader := installTestMetrics(t)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					ID json.RawMessage `json:"id"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}

				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, strings.ReplaceAll(tt.body, "{{id}}", string(req.ID)))
			}))
			t.Cleanup(srv.Close)

			client, err := New(chainIDEth, srv.URL, "", routerAddress)
			require.NoError(t, err)
			require.IsType(t, &meteredClient{}, client.eth, "New must install the metered client")

			// ACT
			err = tt.call(client.eth)

			// ASSERT
			if tt.result == "ok" {
				if !errors.Is(err, ethereum.NotFound) {
					require.NoError(t, err)
				}
			} else {
				require.Error(t, err)
			}
			point := requireSingleOperation(t, reader)
			assert.ElementsMatch(t, []attribute.KeyValue{
				attribute.String("operation", tt.operation),
				attribute.String("chain_id", chainIDEth),
				attribute.String("result", tt.result),
				attribute.String("code", tt.code),
			}, point.Attributes.ToSlice())
		})
	}
}

// installTestMetrics routes the package-level instrumentation New installs to
// a test reader. Wrapper unit tests inject instrumentation directly instead.
func installTestMetrics(t *testing.T) *sdkmetric.ManualReader {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	instruments, err := newInstrumentation(provider.Meter("test"))
	require.NoError(t, err)

	previous := metrics
	metrics = instruments
	t.Cleanup(func() {
		metrics = previous
		require.NoError(t, provider.Shutdown(context.Background()))
	})

	return reader
}
