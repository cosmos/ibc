// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/cosmos/ibc/cli/internal/otel"
)

func TestMetricsTransport(t *testing.T) {
	t.Run("recordsJSONRPCMethodAndChainID", func(t *testing.T) {
		// ARRANGE
		reader := setupTestMetrics(t)
		var (
			seenBody []byte
			readErr  error
		)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			seenBody, readErr = io.ReadAll(r.Body)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1"}`))
		}))
		t.Cleanup(srv.Close)

		transport := newMetricsTransport("1", http.DefaultTransport)
		client := &http.Client{Transport: transport}
		reqBody := []byte(`{"jsonrpc":"2.0","id":1,"method":"eth_getBlockByNumber","params":["latest",false]}`)
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL, bytes.NewReader(reqBody))
		require.NoError(t, err)

		// ACT
		resp, err := client.Do(req)

		// ASSERT
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })
		require.NoError(t, readErr)
		assert.Equal(t, reqBody, seenBody)

		histogram := collectOperationHistogram(t, reader)
		require.Len(t, histogram.DataPoints, 1)
		assert.Equal(t, uint64(1), histogram.DataPoints[0].Count)
		assert.GreaterOrEqual(t, histogram.DataPoints[0].Sum, float64(0))
		assert.ElementsMatch(t, []attribute.KeyValue{
			attribute.String("operation", "eth_getBlockByNumber"),
			attribute.String("chain_id", "1"),
			otel.AttrResult.Int(http.StatusOK),
		}, histogram.DataPoints[0].Attributes.ToSlice())
	})

	t.Run("recordsHTTPStatusCode", func(t *testing.T) {
		for _, tt := range []struct {
			name       string
			statusCode int
		}{
			{
				name:       "ok",
				statusCode: http.StatusOK,
			},
			{
				name:       "serverError",
				statusCode: http.StatusInternalServerError,
			},
			{
				name:       "rateLimited",
				statusCode: http.StatusTooManyRequests,
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				// ARRANGE
				reader := setupTestMetrics(t)
				transport := newMetricsTransport("1", roundTripFunc(func(*http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: tt.statusCode,
						Body:       io.NopCloser(bytes.NewReader(nil)),
					}, nil
				}))
				req, err := http.NewRequestWithContext(
					t.Context(),
					http.MethodPost,
					"http://example.invalid",
					bytes.NewReader([]byte(`{"jsonrpc":"2.0","method":"eth_blockNumber","id":1}`)),
				)
				require.NoError(t, err)

				// ACT
				resp, err := transport.RoundTrip(req)

				// ASSERT
				require.NoError(t, err)
				t.Cleanup(func() { _ = resp.Body.Close() })
				assert.Equal(t, tt.statusCode, resp.StatusCode)

				histogram := collectOperationHistogram(t, reader)
				require.Len(t, histogram.DataPoints, 1)
				assert.ElementsMatch(t, []attribute.KeyValue{
					attribute.String("operation", "eth_blockNumber"),
					attribute.String("chain_id", "1"),
					otel.AttrResult.Int(tt.statusCode),
				}, histogram.DataPoints[0].Attributes.ToSlice())
			})
		}
	})

	t.Run("recordsDurationOnTransportError", func(t *testing.T) {
		// ARRANGE
		reader := setupTestMetrics(t)
		transport := newMetricsTransport("11155111", roundTripFunc(func(*http.Request) (*http.Response, error) {
			time.Sleep(2 * time.Millisecond)
			return nil, errors.New("dial failed")
		}))
		req, err := http.NewRequestWithContext(
			t.Context(),
			http.MethodPost,
			"http://example.invalid",
			bytes.NewReader([]byte(`{"jsonrpc":"2.0","method":"eth_call","id":1}`)),
		)
		require.NoError(t, err)

		// ACT
		resp, err := transport.RoundTrip(req)
		if resp != nil {
			t.Cleanup(func() { _ = resp.Body.Close() })
		}

		// ASSERT
		require.Error(t, err)
		histogram := collectOperationHistogram(t, reader)
		require.Len(t, histogram.DataPoints, 1)
		assert.Equal(t, uint64(1), histogram.DataPoints[0].Count)
		assert.GreaterOrEqual(t, histogram.DataPoints[0].Sum, float64(2))
		assert.ElementsMatch(t, []attribute.KeyValue{
			attribute.String("operation", "eth_call"),
			attribute.String("chain_id", "11155111"),
			otel.AttrResult.Int(-1),
		}, histogram.DataPoints[0].Attributes.ToSlice())
	})

	t.Run("skipsNonJSONRPCBodies", func(t *testing.T) {
		// ARRANGE
		reader := setupTestMetrics(t)
		var seenBody []byte
		transport := newMetricsTransport("1", roundTripFunc(func(r *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			seenBody = body
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(nil))}, nil
		}))

		for _, body := range [][]byte{
			[]byte(`{"method":"eth_call"}`),
			[]byte(`[{"jsonrpc":"2.0","method":"eth_call","id":1}]`),
			[]byte(`not-json`),
		} {
			req, err := http.NewRequestWithContext(
				t.Context(),
				http.MethodPost,
				"http://example.invalid",
				bytes.NewReader(body),
			)
			require.NoError(t, err)

			// ACT
			resp, err := transport.RoundTrip(req)

			// ASSERT
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			assert.Equal(t, body, seenBody)
		}

		var data metricdata.ResourceMetrics
		require.NoError(t, reader.Collect(t.Context(), &data))
		assert.Empty(t, data.ScopeMetrics)
	})
}

func collectOperationHistogram(t *testing.T, reader *sdkmetric.ManualReader) metricdata.Histogram[float64] {
	t.Helper()

	var data metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(t.Context(), &data))
	require.Len(t, data.ScopeMetrics, 1)
	require.Len(t, data.ScopeMetrics[0].Metrics, 1)
	assert.Equal(t, "evm_operation_dur", data.ScopeMetrics[0].Metrics[0].Name)
	assert.Equal(t, "ms", data.ScopeMetrics[0].Metrics[0].Unit)

	histogram, ok := data.ScopeMetrics[0].Metrics[0].Data.(metricdata.Histogram[float64])
	require.True(t, ok)

	return histogram
}

func setupTestMetrics(t *testing.T) *sdkmetric.ManualReader {
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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
