// SPDX-License-Identifier: Apache-2.0

package dispatch

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel/metric"

	"github.com/cosmos/ibc/cli/internal/otel"
	"github.com/cosmos/ibc/cli/internal/relay/processors"
	"github.com/cosmos/ibc/cli/internal/store"
)

// Excessive relay latency thresholds. Hard-coded for now;
// can be moved to config in the future.
const (
	// excessiveRelayLatency is how long a leg may stay pending before it counts
	// as excessive. All configured chains are EVM today.
	excessiveRelayLatency = 60 * time.Minute
	// sourceFinalityDelay is the minimum age of the source tx before a missing
	// timeout relay is considered excessive, giving the send time to finalize.
	sourceFinalityDelay = 15 * time.Minute
	// timeoutExcessiveDelay is how long past the packet timeout a timeout relay
	// may lag before it is considered excessive.
	timeoutExcessiveDelay = 5 * time.Minute
)

// Packet relay legs; values match the processors relay types.
const (
	legSendToRecv    = "send_to_recv"
	legSendToTimeout = "send_to_timeout"
	legRecvToAck     = "recv_to_ack"
)

type instrumentation struct {
	PacketsPending        metric.Int64Gauge
	ExcessiveRelayLatency metric.Int64Counter

	routesFromPrevCall map[processors.Route]struct{}
	mu                 sync.Mutex
}

var metrics = otel.RegisterMetrics("relayer", newInstrumentation)

func newInstrumentation(m metric.Meter) (*instrumentation, error) {
	packetsPending, err := m.Int64Gauge("packets_pending")
	if err != nil {
		return nil, err
	}

	excessiveRelayLatency, err := m.Int64Counter("excessive_relay_latency_total")
	if err != nil {
		return nil, err
	}

	return &instrumentation{
		PacketsPending:        packetsPending,
		ExcessiveRelayLatency: excessiveRelayLatency,
		routesFromPrevCall:    make(map[processors.Route]struct{}),
	}, nil
}

func (m *instrumentation) packetsPending(ctx context.Context, routes []processors.Route, packets []store.Packet) {
	// map route => count, seeded with configured routes
	// so routes without any packets yet still report 0
	counts := make(map[processors.Route]int64, len(packets)+len(routes))

	for _, route := range routes {
		counts[route] = 0
	}

	for _, packet := range packets {
		key := processors.Route{
			SourceChainID:       packet.SourceChainID,
			SourceClientID:      packet.PacketSourceClientID,
			DestinationChainID:  packet.DestinationChainID,
			DestinationClientID: packet.PacketDestinationClientID,
		}

		counts[key]++
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for route, count := range counts {
		m.recordPending(ctx, route, count)
	}

	// drop to zero metrics for routes from prev. call that are no longer present
	for route := range m.routesFromPrevCall {
		if _, ok := counts[route]; !ok {
			m.recordPending(ctx, route, 0)
		}
	}

	// update the map for the next call
	m.routesFromPrevCall = make(map[processors.Route]struct{}, len(counts))
	for route := range counts {
		m.routesFromPrevCall[route] = struct{}{}
	}
}

func (m *instrumentation) recordPending(ctx context.Context, route processors.Route, count int64) {
	m.PacketsPending.Record(ctx, count, otel.WithAttributes(
		otel.AttrChainID.String(route.SourceChainID),
		otel.AttrDestChainID.String(route.DestinationChainID),
		otel.AttrClientID.String(route.SourceClientID),
		otel.AttrDestClientID.String(route.DestinationClientID),
	))
}

// excessiveRelayLatency counts packets that have been pending past the
// threshold for their current relay leg. Unlike packetsPending this is an event
// counter: a packet still stuck on the next poll increments again, so alerts
// should be built on rate().
func (m *instrumentation) excessiveRelayLatency(ctx context.Context, packets []store.Packet) {
	now := time.Now()

	for _, packet := range packets {
		leg, ok := excessiveLatencyLeg(packet, now)
		if !ok {
			continue
		}

		m.ExcessiveRelayLatency.Add(ctx, 1, otel.WithAttributes(
			otel.AttrChainID.String(packet.SourceChainID),
			otel.AttrDestChainID.String(packet.DestinationChainID),
			otel.AttrClientID.String(packet.PacketSourceClientID),
			otel.AttrDestClientID.String(packet.PacketDestinationClientID),
			otel.AttrType.String(leg),
		))
	}
}

func excessiveLatencyLeg(p store.Packet, now time.Time) (string, bool) {
	switch p.Status {
	case store.RelayStatusCompleteWithAck,
		store.RelayStatusCompleteWithWriteAckError,
		store.RelayStatusCompleteWithTimeout,
		store.RelayStatusFailed:
		return "", false
	}

	// Recv is the source of truth for whether the packet can still time out.
	// WriteAckTxHash can stay nil after a recv if write-ack lookup fails.
	if p.RecvTxHash != nil {
		start := p.WriteAckTxTime
		if start == nil {
			start = p.RecvTxTime
		}
		if start != nil && now.Sub(*start) > excessiveRelayLatency {
			return legRecvToAck, true
		}

		return "", false
	}

	if now.After(p.PacketTimeoutTimestamp) {
		if now.Sub(p.PacketTimeoutTimestamp) < timeoutExcessiveDelay {
			return "", false
		}

		// wait for the source tx to finalize before alerting
		if now.Sub(p.SourceTxTime) < sourceFinalityDelay {
			return "", false
		}

		return legSendToTimeout, true
	}

	if now.Sub(p.SourceTxTime) > excessiveRelayLatency {
		return legSendToRecv, true
	}

	return "", false
}
