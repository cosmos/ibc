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

	relayDuration, err := m.Float64Histogram("relay_duration_seconds", otel.UnitSeconds())
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
	for _, leg := range completedRelayLegs(tr) {
		var (
			kind  = leg.kind
			attrs = relayAttributes(tr, kind)
		)

		m.RelaysCompleted.Add(ctx, 1, attrs)
		if leg.startedAt == nil || leg.finishedAt == nil {
			continue
		}

		duration := leg.finishedAt.Sub(*leg.startedAt)
		m.RelayDuration.Record(ctx, duration.Seconds(), attrs)
	}
}

func (m *instrumentation) transactionRetry(ctx context.Context, tr *Transfer, kind relayType) {
	m.TransactionRetries.Add(ctx, 1, relayAttributes(tr, kind))
}

func relayAttributes(tr *Transfer, kind relayType) metric.MeasurementOption {
	return otel.WithAttributes(
		otel.AttrChainID.String(tr.SourceChainID),
		otel.AttrDestChainID.String(tr.DestinationChainID),
		otel.AttrClientID.String(tr.PacketSourceClientID),
		otel.AttrDestClientID.String(tr.PacketDestinationClientID),
		otel.AttrType.String(string(kind)),
	)
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
