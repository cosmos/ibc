// SPDX-License-Identifier: Apache-2.0

package dispatch

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel/metric"

	"github.com/cosmos/ibc/cli/internal/otel"
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

type routeKey struct {
	chainID      string
	destChainID  string
	clientID     string
	destClientID string
}

type instrumentation struct {
	PacketsPending        metric.Int64Gauge
	ExcessiveRelayLatency metric.Int64Counter

	routesFromPrevCall map[routeKey]struct{}
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
		routesFromPrevCall:    make(map[routeKey]struct{}),
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

	if p.WriteAckTxHash == nil {
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

	if p.WriteAckTxTime != nil && now.Sub(*p.WriteAckTxTime) > excessiveRelayLatency {
		return legRecvToAck, true
	}

	return "", false
}
