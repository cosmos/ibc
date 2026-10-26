// SPDX-License-Identifier: Apache-2.0

package processors

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/cosmos/ibc/cli/internal/store"
	"github.com/cosmos/ibc/cli/internal/tests/mocks"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

type executionStore struct {
	cleared int
}

func (s *executionStore) ClearPacketRecvTx(context.Context, store.PacketKey) error {
	s.cleared++
	return nil
}

func (s *executionStore) ClearPacketAckTx(context.Context, store.PacketKey) error {
	s.cleared++
	return nil
}

func (s *executionStore) ClearPacketTimeoutTx(context.Context, store.PacketKey) error {
	s.cleared++
	return nil
}

func (*executionStore) UpdatePacketWriteAck(context.Context, store.PacketKey, store.WriteAck) error {
	return nil
}

func executionTransfer() *Transfer {
	hash := "0xbatch"
	now := time.Now()
	return NewTransfer(store.Packet{
		SourceChainID: "1", DestinationChainID: "2", PacketSourceClientID: "source", PacketDestinationClientID: "dest",
		PacketSequenceNumber: 1, RecvTxHash: &hash, RecvTxTime: &now, AckTxHash: &hash, AckTxTime: &now,
		TimeoutTxHash: &hash, TimeoutTxTime: &now,
	}, slog.Default())
}

func TestRetryExecutionMetrics(t *testing.T) {
	for _, kind := range []v2.RelayKind{v2.RelayKindRecv, v2.RelayKindAck, v2.RelayKindTimeout} {
		for _, tc := range []struct {
			name     string
			retry    bool
			err      error
			executed bool
			result   string
		}{
			{name: "pending", err: v2.ErrTxNotFound},
			{name: "rpc failure", err: assert.AnError},
			{name: "expired missing receipt", retry: true},
			{name: "reverted", retry: true, err: v2.ErrTxReverted, result: executionReverted},
			{name: "executed", executed: true, result: executionSucceeded},
			{name: "missing event", result: executionMissing},
		} {
			t.Run(kind.String()+"/"+tc.name, func(t *testing.T) {
				if kind == v2.RelayKindRecv && (tc.result == executionSucceeded || tc.result == executionMissing) {
					t.Skip("recv outcomes are observed in WaitForWriteAck")
				}
				suite := newMetricsTestSuite(t)
				suite.install(t)
				tr := executionTransfer()
				s := &executionStore{}
				submitter := mocks.NewMockTxSubmitter(t)
				submitter.EXPECT().ShouldRetry(mock.Anything, "0xbatch", mock.Anything).Return(tc.retry, tc.err).Twice()
				client := mocks.NewMockClient(t)
				if !tc.retry && tc.err == nil {
					client.EXPECT().PacketRelayExecuted(mock.Anything, "0xbatch", kind, uint64(1), "source", "dest").
						Return(tc.executed, nil).Twice()
				}
				route := Route{
					SourceChainID:       "1",
					DestinationChainID:  "2",
					SourceClientID:      "source",
					DestinationClientID: "dest",
				}
				var p interface {
					Process(context.Context, *Transfer) (*Transfer, error)
				}
				chainID := "1"
				switch kind {
				case v2.RelayKindRecv:
					chainID = "2"
					p = NewRetryRecvPacket(submitter, s, route)
				case v2.RelayKindAck:
					p = NewRetryAckPacket(staticChains{"1": client}, submitter, s, route)
				case v2.RelayKindTimeout:
					p = NewRetryTimeoutPacket(staticChains{"1": client}, submitter, s, route)
				}
				metrics.txSubmitted(suite.ctx, chainID, "client", "0xbatch")
				for range 2 {
					_, err := p.Process(suite.ctx, tr)
					if tc.executed {
						require.NoError(t, err)
					} else {
						require.Error(t, err)
					}
				}
				counts := executionCounts(t, suite)
				if tc.result == "" {
					assert.Empty(t, counts)
				} else {
					assert.Equal(t, map[string]int64{tc.result: 1}, counts)
				}
				if tc.retry || tc.result == executionMissing {
					assert.Equal(t, 2, s.cleared)
					var data metricdata.ResourceMetrics
					require.NoError(t, suite.reader.Collect(suite.ctx, &data))
					sum := findInt64Sum(t, data, "transaction_retries_total")
					require.Len(t, sum.DataPoints, 1)
					assert.Equal(t, int64(1), sum.DataPoints[0].Value, "confirmation must not consume retry accounting")
				} else {
					assert.Zero(t, s.cleared)
				}
			})
		}
	}
}

func TestRecvExecutionMetrics(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status v2.WriteAckStatus
		err    error
		result string
	}{
		{name: "success", status: v2.WriteAckStatusSuccess, result: executionSucceeded},
		{name: "application error", status: v2.WriteAckStatusError, result: executionSucceeded},
		{name: "missing", err: v2.ErrWriteAckNotFoundForPacket, result: executionMissing},
		{name: "decoding error", err: v2.ErrWriteAckDecoding},
		{name: "pending", err: v2.ErrTxNotFound},
		{name: "rpc failure", err: assert.AnError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			suite := newMetricsTestSuite(t)
			suite.install(t)
			tr := executionTransfer()
			client := mocks.NewMockClient(t)
			client.EXPECT().PacketWriteAckStatus(mock.Anything, "0xbatch", uint64(1), "source", "dest").
				Return(tc.status, tc.err).Twice()
			s := &executionStore{}
			p := NewWaitForWriteAck(staticChains{"2": client}, s)
			metrics.txSubmitted(suite.ctx, "2", "dest", "0xbatch")
			metrics.txConfirmed(suite.ctx, "2", "dest", "0xbatch")
			for range 2 {
				_, err := p.Process(suite.ctx, tr)
				if tc.err == nil || errors.Is(tc.err, v2.ErrWriteAckDecoding) {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
				}
			}
			counts := executionCounts(t, suite)
			if tc.result == "" {
				assert.Empty(t, counts)
			} else {
				assert.Equal(t, map[string]int64{tc.result: 1}, counts)
			}
			if tc.result == executionMissing {
				var data metricdata.ResourceMetrics
				require.NoError(t, suite.reader.Collect(suite.ctx, &data))
				sum := findInt64Sum(t, data, "transaction_retries_total")
				assert.Equal(t, int64(1), sum.DataPoints[0].Value)
			}
		})
	}
}

func TestPacketExecutionBatchDeduplication(t *testing.T) {
	suite := newMetricsTestSuite(t)
	var wg sync.WaitGroup
	for seq := uint64(1); seq <= 3; seq++ {
		tr := executionTransfer()
		tr.PacketSequenceNumber = seq
		result := executionSucceeded
		if seq == 3 {
			result = executionMissing
		}
		for range 10 {
			wg.Go(func() { suite.instruments.packetExecution(suite.ctx, tr, "ack", "0xbatch", result) })
		}
	}
	wg.Wait()
	assert.Equal(t, map[string]int64{executionSucceeded: 2, executionMissing: 1}, executionCounts(t, suite))
	tr := executionTransfer()
	suite.instruments.packetExecution(suite.ctx, tr, "ack", "0xretry", executionSucceeded)
	assert.Equal(t, map[string]int64{executionSucceeded: 3, executionMissing: 1}, executionCounts(t, suite))
}

func executionCounts(t *testing.T, suite *metricsTestSuite) map[string]int64 {
	t.Helper()
	var data metricdata.ResourceMetrics
	require.NoError(t, suite.reader.Collect(suite.ctx, &data))
	counts := make(map[string]int64)
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name != "relay_packet_execution_total" {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok)
			for _, point := range sum.DataPoints {
				result, found := point.Attributes.Value("result")
				require.True(t, found)
				counts[result.AsString()] += point.Value
			}
		}
	}
	return counts
}
