// SPDX-License-Identifier: Apache-2.0

package relayerload

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	relayerv2 "github.com/cosmos/ibc/cli/api/v2/relayer"
)

// progressEvery spaces the sweep's progress logs.
const progressEvery = 10 * time.Second

// trackedPacket is one sent packet and the relayer's latest status for it.
type trackedPacket struct {
	key packetKey
	// sendBlock is the source block that included the transfer.
	sendBlock uint64
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
// source client.
type listFunc func(ctx context.Context) ([]*relayerv2.PacketStatus, error)

// sweep tracks every packet from sent and, every poll, lists the relayer's
// statuses and records those of tracked packets. It returns once sent is
// closed and every tracked packet is terminal. It gives up when relaying
// stalls, no packet sent and none newly terminal for stall, or when ctx ends.
// Packets still non-terminal when sweep returns are unresolved.
func sweep(
	ctx context.Context,
	label string,
	poll, stall time.Duration,
	list listFunc,
	sent <-chan sentPacket,
) ([]*trackedPacket, error) {
	var (
		tracked = map[packetKey]*trackedPacket{}
		order   []*trackedPacket
		settled int
		listErr error
		logged  = time.Now()
	)
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	stalled := time.NewTimer(stall)
	defer stalled.Stop()
	// unresolved explains why sweep gave up with packets still in flight.
	unresolved := func(cause error) error {
		err := fmt.Errorf("%d of %d packets not terminal: %w", len(order)-settled, len(order), cause)
		if listErr != nil {
			err = fmt.Errorf("%w (last listing error: %w)", err, listErr)
		}
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return order, unresolved(ctx.Err())
		case packet, ok := <-sent:
			if !ok {
				sent = nil
				continue
			}
			p := &trackedPacket{key: packet.key, sendBlock: packet.block}
			tracked[packet.key] = p
			order = append(order, p)
			stalled.Reset(stall)
			continue
		case <-stalled.C:
			return order, unresolved(fmt.Errorf("no relay progress for %s", stall))
		case <-ticker.C:
		}

		statuses, err := list(ctx)
		if err != nil {
			listErr = err
			slog.WarnContext(ctx, "list relayer packets", "direction", label, "err", err)
			continue
		}
		for _, status := range statuses {
			key := packetKey{clientID: status.GetSourceClientId(), sequence: status.GetSequenceNumber()}
			if p, ok := tracked[key]; ok {
				p.status = status
			}
		}
		terminal := 0
		for _, p := range order {
			if p.terminal() {
				terminal++
			}
		}
		if terminal > settled {
			settled = terminal
			stalled.Reset(stall)
		}
		if sent == nil && settled == len(order) {
			return order, nil
		}
		if time.Since(logged) >= progressEvery {
			logged = time.Now()
			slog.InfoContext(ctx, "relay progress", "direction", label, "tracked", len(order), "settled", settled)
		}
	}
}
