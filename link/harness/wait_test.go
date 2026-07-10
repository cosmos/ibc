package harness

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"
	"github.com/cosmos/ibc/link/harness/topology"
)

// statusFunc adapts a func to onchain.StatusSource so wait tests script the status API per call.
type statusFunc func(ctx context.Context, q wire.StatusQuery) (*wire.Status, error)

func (f statusFunc) Status(ctx context.Context, q wire.StatusQuery) (*wire.Status, error) {
	return f(ctx, q)
}

// scriptStatus returns a StatusSource that serves each step once (the last repeats forever). A nil
// *wire.Status step serves an error instead, exercising the transient-retry path.
func scriptStatus(steps ...*wire.Status) statusFunc {
	var mu sync.Mutex
	i := 0
	return func(_ context.Context, _ wire.StatusQuery) (*wire.Status, error) {
		mu.Lock()
		defer mu.Unlock()
		s := steps[i]
		if i < len(steps)-1 {
			i++
		}
		if s == nil {
			return nil, errors.New("scripted transient error")
		}
		return s, nil
	}
}

func packetIn(state wire.PacketState) *wire.Status {
	return &wire.Status{Packets: []wire.PacketStatus{{
		PacketID: "p-1",
		RouteID:  "r-1",
		Sequence: 7,
		State:    state,
	}}}
}

// fastProfile keeps the wait tests sub-second: 24 settle observations at 5ms inside a 500ms budget.
func fastProfile() topology.TimingProfile {
	return topology.TimingProfile{
		CompletionBudget: 500 * time.Millisecond,
		SettleWindow:     120 * time.Millisecond,
		PollInterval:     5 * time.Millisecond,
	}
}

func TestWaitPacketStateReturnsMatch(t *testing.T) {
	src := scriptStatus(packetIn(wire.PacketPending), packetIn(wire.PacketPending), packetIn(wire.PacketComplete))
	ps, err := waitPacketState(t.Context(), src, "p-1", wire.PacketComplete, fastProfile())
	require.NoError(t, err)
	require.Equal(t, wire.PacketComplete, ps.State)
	require.Equal(t, uint64(7), ps.Sequence)
}

func TestWaitPacketStateRetriesTransientErrors(t *testing.T) {
	src := scriptStatus(nil, nil, packetIn(wire.PacketComplete))
	_, err := waitPacketState(t.Context(), src, "p-1", wire.PacketComplete, fastProfile())
	require.NoError(t, err)
}

func TestWaitPacketStateTimeoutNamesStuckState(t *testing.T) {
	prof := fastProfile()
	prof.CompletionBudget = 50 * time.Millisecond
	_, err := waitPacketState(t.Context(), scriptStatus(packetIn(wire.PacketPending)), "p-1", wire.PacketComplete, prof)
	require.Error(t, err)
	require.ErrorContains(t, err, `packet p-1 in state "pending"`)
}

func TestWaitPacketStateTimeoutNamesMissingPacket(t *testing.T) {
	prof := fastProfile()
	prof.CompletionBudget = 50 * time.Millisecond
	empty := &wire.Status{}
	_, err := waitPacketState(t.Context(), scriptStatus(empty), "p-1", wire.PacketComplete, prof)
	require.Error(t, err)
	require.ErrorContains(t, err, "not present in daemon status")
}

func TestWaitPacketStableHolds(t *testing.T) {
	err := waitPacketStable(t.Context(), scriptStatus(packetIn(wire.PacketPending)), "p-1", wire.PacketPending, fastProfile())
	require.NoError(t, err)
}

func TestWaitPacketStableRejectsFlap(t *testing.T) {
	// The packet flips to complete partway through the settle window; the assertion must fail, not
	// "recover" — a stability check holds at every sample.
	src := scriptStatus(
		packetIn(wire.PacketPending),
		packetIn(wire.PacketPending),
		packetIn(wire.PacketComplete),
	)
	err := waitPacketStable(t.Context(), src, "p-1", wire.PacketPending, fastProfile())
	require.Error(t, err)
	require.ErrorContains(t, err, `must remain "pending"`)
}

func TestWaitPacketStableRejectsDisappearance(t *testing.T) {
	src := scriptStatus(packetIn(wire.PacketPending), &wire.Status{})
	err := waitPacketStable(t.Context(), src, "p-1", wire.PacketPending, fastProfile())
	require.Error(t, err)
	require.ErrorContains(t, err, "must stay present")
}
