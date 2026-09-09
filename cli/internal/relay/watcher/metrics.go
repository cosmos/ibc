// SPDX-License-Identifier: Apache-2.0

package watcher

import (
	"context"

	"go.opentelemetry.io/otel/metric"

	"github.com/cosmos/ibc/cli/internal/otel"
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

func (m *instrumentation) sendPacket(ctx context.Context, chainID string) {
	m.EventsTotal.Add(ctx, 1, otel.WithAttributes(
		otel.AttrChainID.String(chainID),
		otel.AttrType.String("send_packet"),
	))
}
