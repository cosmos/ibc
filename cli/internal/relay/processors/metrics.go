// SPDX-License-Identifier: Apache-2.0

package processors

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/cosmos/ibc/cli/internal/otel"
	"github.com/cosmos/ibc/cli/internal/store"
)

type relayType string

const (
	relayTypeSendToRecv    relayType = "send_to_recv"
	relayTypeSendToTimeout relayType = "send_to_timeout"
	relayTypeRecvToAck     relayType = "recv_to_ack"
)

type instrumentation struct {
	RelaysCompleted       metric.Int64Counter
	RelayDuration         metric.Float64Histogram
	TransactionRetries    metric.Int64Counter
	TransactionsSubmitted metric.Int64Counter
	TransactionsConfirmed metric.Int64Counter

	submittedTransactions sync.Map
}

type completedRelayLeg struct {
	kind       relayType
	startedAt  *time.Time
	finishedAt *time.Time
}

var metrics = otel.RegisterMetrics("relayer", newInstrumentation)

func newInstrumentation(m metric.Meter) (*instrumentation, error) {
	relaysCompleted, err := m.Int64Counter("relays_completed_total")
	if err != nil {
		return nil, err
	}

	relayDuration, err := m.Float64Histogram("relay_duration", otel.UnitSeconds())
	if err != nil {
		return nil, err
	}

	transactionRetries, err := m.Int64Counter("transaction_retries_total")
	if err != nil {
		return nil, err
	}

	transactionsSubmitted, err := m.Int64Counter("transactions_submitted_total")
	if err != nil {
		return nil, err
	}

	transactionsConfirmed, err := m.Int64Counter("transactions_confirmed_total")
	if err != nil {
		return nil, err
	}

	return &instrumentation{
		RelaysCompleted:       relaysCompleted,
		RelayDuration:         relayDuration,
		TransactionRetries:    transactionRetries,
		TransactionsSubmitted: transactionsSubmitted,
		TransactionsConfirmed: transactionsConfirmed,
	}, nil
}

func (m *instrumentation) relayCompleted(ctx context.Context, tr *Transfer) {
	for _, leg := range completedRelayLegs(tr) {
		kind := leg.kind
		attrs := txAttributes(tr, otel.AttrType.String(string(kind)))

		m.RelaysCompleted.Add(ctx, 1, attrs)
		if leg.startedAt == nil || leg.finishedAt == nil {
			continue
		}

		duration := leg.finishedAt.Sub(*leg.startedAt)
		m.RelayDuration.Record(ctx, duration.Seconds(), attrs)
	}
}

func (m *instrumentation) txSubmitted(ctx context.Context, chainID, clientID, txHash string) {
	key := txKey(chainID, txHash)
	m.submittedTransactions.Store(key, struct{}{})
	m.TransactionsSubmitted.Add(ctx, 1, txMetricAttributes(chainID, clientID))
}

func (m *instrumentation) txConfirmed(ctx context.Context, chainID, clientID, txHash string) {
	key := txKey(chainID, txHash)
	if _, submitted := m.submittedTransactions.LoadAndDelete(key); !submitted {
		return
	}

	m.TransactionsConfirmed.Add(ctx, 1, txMetricAttributes(chainID, clientID))
}

func (m *instrumentation) txRetry(ctx context.Context, tr *Transfer, kind relayType) {
	m.TransactionRetries.Add(ctx, 1, txAttributes(tr, otel.AttrType.String(string(kind))))
}

func txKey(chainID, txHash string) string {
	return chainID + ":" + txHash
}

// chain receiving the transaction, client updated by that transaction
func txMetricAttributes(chainID, clientID string) metric.MeasurementOption {
	return otel.WithAttributes(
		otel.AttrChainID.String(chainID),
		otel.AttrClientID.String(clientID),
	)
}

func txAttributes(tr *Transfer, extra ...attribute.KeyValue) metric.MeasurementOption {
	attrs := []attribute.KeyValue{
		otel.AttrChainID.String(tr.SourceChainID),
		otel.AttrDestChainID.String(tr.DestinationChainID),
		otel.AttrClientID.String(tr.PacketSourceClientID),
		otel.AttrDestClientID.String(tr.PacketDestinationClientID),
	}

	attrs = append(attrs, extra...)

	return otel.WithAttributes(attrs...)
}

func completedRelayLegs(tr *Transfer) []completedRelayLeg {
	switch tr.Status {
	case store.RelayStatusCompleteWithAck:
		return []completedRelayLeg{
			{
				kind:       relayTypeSendToRecv,
				startedAt:  &tr.SourceTxTime,
				finishedAt: tr.RecvTxTime,
			},
			{
				kind:       relayTypeRecvToAck,
				startedAt:  tr.RecvTxTime,
				finishedAt: tr.AckTxTime,
			},
		}
	case store.RelayStatusCompleteWithTimeout:
		return []completedRelayLeg{
			{
				kind:       relayTypeSendToTimeout,
				startedAt:  &tr.SourceTxTime,
				finishedAt: tr.TimeoutTxTime,
			},
		}
	default:
		return nil
	}
}
