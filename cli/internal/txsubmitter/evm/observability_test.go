// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strconv"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

func TestSubmissionAttempts(t *testing.T) {
	for _, tc := range []struct {
		name, result string
		prepare      func(*TxSubmitter, *testRPC)
	}{
		{name: "accepted", result: "ok"},
		{name: "fee replacement", result: "ok", prepare: func(_ *TxSubmitter, rpc *testRPC) {
			rpc.send = func(*types.Transaction) error {
				if len(rpc.sent) == 1 {
					return errors.New("replacement transaction underpriced")
				}
				return nil
			}
		}},
		{name: "capacity", result: "deferred", prepare: func(c *TxSubmitter, _ *testRPC) { c.nextNonce = maxInFlight }},
		{name: "gas estimation", result: "error", prepare: func(_ *TxSubmitter, rpc *testRPC) { rpc.gasErr = assert.AnError }},
		{name: "broadcast", result: "error", prepare: func(_ *TxSubmitter, rpc *testRPC) {
			rpc.send = func(*types.Transaction) error { return assert.AnError }
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, reader := installTestMetrics(t)
			c, rpc := newSubmitter(t)
			if tc.prepare != nil {
				tc.prepare(c, rpc)
			}
			_, err := c.Submit(t.Context(), testIntent())
			if tc.result == "ok" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			var data metricdata.ResourceMetrics
			require.NoError(t, reader.Collect(t.Context(), &data))
			for _, scope := range data.ScopeMetrics {
				for _, m := range scope.Metrics {
					if m.Name != "txsubmitter_attempts_total" {
						continue
					}
					sum, ok := m.Data.(metricdata.Sum[int64])
					require.True(t, ok)
					require.Len(t, sum.DataPoints, 1)
					assert.Equal(t, int64(1), sum.DataPoints[0].Value)
					assert.Equal(t, attribute.NewSet(
						attribute.String("chain_id", chainIDEth), attribute.String("wallet", c.address.Hex()),
						attribute.String("result", tc.result)), sum.DataPoints[0].Attributes)
					return
				}
			}
			t.Fatal("submission attempts missing")
		})
	}
}

func TestTransactionDiagnosticLogs(t *testing.T) {
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	c, rpc := newSubmitter(t)
	rpc.send = func(*types.Transaction) error {
		if len(rpc.sent) == 1 {
			return errors.New("replacement transaction underpriced")
		}
		return nil
	}
	submission, err := c.Submit(t.Context(), testIntent())
	require.NoError(t, err)
	require.Len(t, rpc.sent, 2)

	var events []map[string]any
	decode := func() {
		decoder := json.NewDecoder(&buf)
		decoder.UseNumber()
		for decoder.More() {
			var event map[string]any
			require.NoError(t, decoder.Decode(&event))
			events = append(events, event)
		}
	}
	decode()
	require.Len(t, events, 2)
	assert.Equal(t, "Transaction broadcast rejected or unconfirmed", events[0]["msg"])
	assert.Equal(t, "Transaction submitted", events[1]["msg"])
	assert.Equal(t, "INFO", events[1]["level"])
	assert.Equal(t, submission.TxHash, events[1]["txHash"])
	assert.Equal(t, c.address.Hex(), events[1]["sender"])
	assert.Equal(t, json.Number("0"), events[1]["nonce"])
	assert.Equal(t, json.Number(strconv.FormatUint(rpc.gas, 10)), events[1]["gasLimit"])
	assert.Equal(t, rpc.sent[1].GasFeeCap().String(), events[1]["maxFeePerGasWei"])
	assert.Equal(t, rpc.sent[1].GasTipCap().String(), events[1]["maxPriorityFeePerGasWei"])
	assert.Equal(t, "100", events[1]["baseFeePerGasWei"])

	_, err = c.ShouldRetry(t.Context(), submission.TxHash, time.Now())
	require.ErrorIs(t, err, v2.ErrTxNotFound)
	assert.Empty(t, buf.String())
}
