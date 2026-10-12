// SPDX-License-Identifier: Apache-2.0

package relayerload

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"time"

	relayerv2 "github.com/cosmos/ibc/cli/api/v2/relayer"
)

// progressEvery spaces the sweep's progress logs.
const progressEvery = 10 * time.Second

// trackedPacket is one sent packet and the relayer's latest status for it.
type trackedPacket struct {
	sentPacket
	// status is nil until the relayer lists the packet.
	status *relayerv2.PacketStatus
}

func (p *trackedPacket) state() relayerv2.PacketState { return p.status.GetState() }

func (p *trackedPacket) terminal() bool {
	switch p.state() {
	case relayerv2.PacketState_PACKET_STATE_SUCCEEDED,
		relayerv2.PacketState_PACKET_STATE_TIMED_OUT,
		relayerv2.PacketState_PACKET_STATE_REJECTED,
		relayerv2.PacketState_PACKET_STATE_RELAY_FAILED:
		return true
	default:
		return false
	}
}

// listFunc lists the relayer's status of every packet from the direction's
// source client with at least fromSequence.
type listFunc func(ctx context.Context, fromSequence uint64) ([]*relayerv2.PacketStatus, error)

// sweep tracks every packet put in sent and, every poll, lists the relayer's
// statuses from the oldest tracked packet that is not terminal, so settled
// packets are not listed again, and records those of tracked packets. It
// returns once sent is closed and every tracked packet is terminal. It gives
// up when relaying stalls, packets outstanding but none newly terminal for
// stall, or when ctx ends. Packets still non-terminal when sweep returns are
// unresolved.
func sweep(
	ctx context.Context,
	label string,
	poll, stall time.Duration,
	list listFunc,
	sent *inbox,
) ([]*trackedPacket, error) {
	var (
		tracked = map[packetKey]*trackedPacket{}
		order   []*trackedPacket
		// unsettled holds the tracked packets not yet terminal.
		unsettled  []*trackedPacket
		closed     bool
		listErr    error
		progressed = time.Now()
		logged     = time.Now()
	)
	// track takes the packets sent since the last poll.
	track := func() {
		var packets []sentPacket
		packets, closed = sent.take()
		for _, packet := range packets {
			p := &trackedPacket{sentPacket: packet}
			tracked[packet.key] = p
			order = append(order, p)
			unsettled = append(unsettled, p)
		}
	}
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	// unresolved explains why sweep gave up with packets still in flight.
	unresolved := func(cause error) error {
		err := fmt.Errorf("%d of %d packets not terminal: %w", len(unsettled), len(order), cause)
		if listErr != nil {
			err = fmt.Errorf("%w (last listing error: %w)", err, listErr)
		}
		return err
	}
	for {
		select {
		case <-ctx.Done():
			track()
			return order, unresolved(context.Cause(ctx))
		case <-ticker.C:
		}
		track()

		if len(unsettled) > 0 {
			from := unsettled[0].key.sequence
			for _, p := range unsettled[1:] {
				from = min(from, p.key.sequence)
			}
			if statuses, err := list(ctx, from); err != nil {
				listErr = err
				slog.WarnContext(ctx, "list relayer packets", "direction", label, "err", err)
			} else {
				listErr = nil
				for _, status := range statuses {
					key := packetKey{clientID: status.GetSourceClientId(), sequence: status.GetSequenceNumber()}
					if p, ok := tracked[key]; ok {
						p.status = status
					}
				}
				before := len(unsettled)
				unsettled = slices.DeleteFunc(unsettled, (*trackedPacket).terminal)
				if len(unsettled) < before {
					progressed = time.Now()
				}
			}
		}
		// Nothing outstanding is no stall, however long sending pauses.
		if len(unsettled) == 0 {
			if closed {
				return order, nil
			}
			progressed = time.Now()
		}
		// A slow listing delays this check but never hides progress it saw.
		if time.Since(progressed) >= stall {
			return order, unresolved(fmt.Errorf("no relay progress for %s", stall))
		}
		if time.Since(logged) >= progressEvery {
			logged = time.Now()
			slog.InfoContext(ctx, "relay progress", "direction", label, "tracked", len(order),
				"settled", len(order)-len(unsettled))
		}
	}
}
