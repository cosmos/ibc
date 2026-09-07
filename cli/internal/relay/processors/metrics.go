// SPDX-License-Identifier: Apache-2.0

package processors

import (
	"context"
	"time"

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
	RelaysCompleted    metric.Int64Counter
	RelayDuration      metric.Float64Histogram
	TransactionRetries metric.Int64Counter
}

var metrics instrumentation

func init() {
	otel.RegisterMetrics("relayer", newInstrumentation, &metrics)
}

func newInstrumentation(m metric.Meter) (*instrumentation, error) {
	relaysCompleted, err := m.Int64Counter("relays_completed_total")
	if err != nil {
		return nil, err
	}

	relayDuration, err := m.Float64Histogram("relay_duration_seconds", metric.WithUnit("s"))
	if err != nil {
		return nil, err
	}

	transactionRetries, err := m.Int64Counter("transaction_retries_total")
	if err != nil {
		return nil, err
	}

	return &instrumentation{
		RelaysCompleted:    relaysCompleted,
		RelayDuration:      relayDuration,
		TransactionRetries: transactionRetries,
	}, nil
}

func (m *instrumentation) relayCompleted(ctx context.Context, tr *Transfer) {
	switch tr.Status {
	case store.RelayStatusCompleteWithTimeout:
		m.RelaysCompleted.Add(ctx, 1, relayAttributes(tr, relayTypeSendToTimeout, true))
	case store.RelayStatusCompleteWithAck:
		m.RelaysCompleted.Add(ctx, 1, relayAttributes(tr, relayTypeSendToRecv, true))
		m.RelaysCompleted.Add(ctx, 1, relayAttributes(tr, relayTypeRecvToAck, true))
	}
}

func (m *instrumentation) relayFinished(ctx context.Context, tr *Transfer) {
	switch {
	case tr.TimeoutTxTime != nil:
		m.relayDuration(ctx, tr, relayTypeSendToTimeout, tr.SourceTxTime, *tr.TimeoutTxTime)
	case tr.AckTxTime != nil:
		if tr.RecvTxTime != nil {
			m.relayDuration(ctx, tr, relayTypeSendToRecv, tr.SourceTxTime, *tr.RecvTxTime)
			m.relayDuration(ctx, tr, relayTypeRecvToAck, *tr.RecvTxTime, *tr.AckTxTime)
		}
	}
}

func (m *instrumentation) relayDuration(
	ctx context.Context,
	tr *Transfer,
	kind relayType,
	from time.Time,
	to time.Time,
) {
	duration := to.Sub(from)
	m.RelayDuration.Record(ctx, duration.Seconds(), relayAttributes(tr, kind, true))
}

func (m *instrumentation) transactionRetry(ctx context.Context, tr *Transfer, kind relayType) {
	m.TransactionRetries.Add(ctx, 1, relayAttributes(tr, kind, false))
}

func relayAttributes(tr *Transfer, kind relayType, clients bool) metric.MeasurementOption {
	if !clients {
		return otel.WithAttributes(
			otel.AttrChainID.String(tr.SourceChainID),
			otel.AttrDestChainID.String(tr.DestinationChainID),
			otel.AttrRelayType.String(string(kind)),
		)
	}

	return otel.WithAttributes(
		otel.AttrChainID.String(tr.SourceChainID),
		otel.AttrDestChainID.String(tr.DestinationChainID),
		otel.AttrClientID.String(tr.PacketSourceClientID),
		otel.AttrDestClientID.String(tr.PacketDestinationClientID),
		otel.AttrRelayType.String(string(kind)),
	)
}
