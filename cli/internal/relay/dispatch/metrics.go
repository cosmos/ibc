// SPDX-License-Identifier: Apache-2.0

package dispatch

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel/metric"

	"github.com/cosmos/ibc/cli/internal/otel"
	"github.com/cosmos/ibc/cli/internal/store"
)

type routeKey struct {
	chainID      string
	destChainID  string
	clientID     string
	destClientID string
}

type instrumentation struct {
	PacketsPending metric.Int64Gauge

	routesFromPrevCall map[routeKey]struct{}
	mu                 sync.Mutex
}

var metrics = otel.RegisterMetrics("relayer", newInstrumentation)

func newInstrumentation(m metric.Meter) (*instrumentation, error) {
	packetsPending, err := m.Int64Gauge("packets_pending")
	if err != nil {
		return nil, err
	}

	return &instrumentation{
		PacketsPending:     packetsPending,
		routesFromPrevCall: make(map[routeKey]struct{}),
	}, nil
}

func (m *instrumentation) packetsPending(ctx context.Context, packets []store.Packet) {
	// map route => count
	counts := make(map[routeKey]int64, len(packets))
	for _, packet := range packets {
		key := routeKey{
			chainID:      packet.SourceChainID,
			destChainID:  packet.DestinationChainID,
			clientID:     packet.PacketSourceClientID,
			destClientID: packet.PacketDestinationClientID,
		}
		counts[key]++
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for key, count := range counts {
		m.recordPending(ctx, key, count)
	}

	// drop to zero metrics for routes from prev. call that are no longer present
	for key := range m.routesFromPrevCall {
		if _, ok := counts[key]; !ok {
			m.recordPending(ctx, key, 0)
		}
	}

	// update the map for the next call
	m.routesFromPrevCall = make(map[routeKey]struct{}, len(counts))
	for key := range counts {
		m.routesFromPrevCall[key] = struct{}{}
	}
}

func (m *instrumentation) recordPending(ctx context.Context, key routeKey, count int64) {
	m.PacketsPending.Record(ctx, count, otel.WithAttributes(
		otel.AttrChainID.String(key.chainID),
		otel.AttrDestChainID.String(key.destChainID),
		otel.AttrClientID.String(key.clientID),
		otel.AttrDestClientID.String(key.destClientID),
	))
}
