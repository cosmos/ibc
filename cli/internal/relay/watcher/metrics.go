// SPDX-License-Identifier: Apache-2.0

package watcher

import (
	"context"

	"go.opentelemetry.io/otel/metric"

	"github.com/cosmos/ibc/cli/internal/otel"
)

type instrumentation struct {
	EventsTotal    metric.Int64Counter
	SubscriptionUp metric.Int64Gauge
}

var metrics = otel.RegisterMetrics("relayer", newInstrumentation)

func newInstrumentation(m metric.Meter) (*instrumentation, error) {
	eventsTotal, err := m.Int64Counter("watcher_events_total")
	if err != nil {
		return nil, err
	}

	subscriptionUp, err := m.Int64Gauge("watcher_subscription_up")
	if err != nil {
		return nil, err
	}
	return &instrumentation{EventsTotal: eventsTotal, SubscriptionUp: subscriptionUp}, nil
}

func (m *instrumentation) subscription(ctx context.Context, chainID string, up bool) {
	var value int64
	if up {
		value = 1
	}
	m.SubscriptionUp.Record(context.WithoutCancel(ctx), value, metric.WithAttributes(otel.AttrChainID.String(chainID)))
}

func (m *instrumentation) sendPacket(ctx context.Context, chainID string) {
	m.EventsTotal.Add(ctx, 1, metric.WithAttributes(
		otel.AttrChainID.String(chainID),
		otel.AttrType.String("send_packet"),
	))
}
