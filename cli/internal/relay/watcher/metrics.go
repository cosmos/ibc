// SPDX-License-Identifier: Apache-2.0

package watcher

import (
	"context"

	"go.opentelemetry.io/otel/metric"

	"github.com/cosmos/ibc/cli/internal/otel"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

type instrumentation struct {
	EventsTotal metric.Int64Counter
}

var metrics = otel.RegisterMetrics("relayer", newInstrumentation)

func newInstrumentation(m metric.Meter) (*instrumentation, error) {
	eventsTotal, err := m.Int64Counter("watcher_events_total")
	if err != nil {
		return nil, err
	}

	return &instrumentation{EventsTotal: eventsTotal}, nil
}

func (m *instrumentation) event(ctx context.Context, chainID string, kind v2.EventKind) {
	eventType, ok := eventType(kind)
	if !ok {
		return
	}

	m.EventsTotal.Add(ctx, 1, otel.WithAttributes(
		otel.AttrChainID.String(chainID),
		otel.AttrType.String(eventType),
	))
}

func eventType(kind v2.EventKind) (string, bool) {
	switch kind {
	case v2.KindSendPacket:
		return "send_packet", true
	case v2.KindWriteAck:
		return "write_ack", true
	default:
		return "", false
	}
}
