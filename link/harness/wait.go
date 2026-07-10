package harness

import (
	"context"
	"fmt"
	"time"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"
	"github.com/cosmos/ibc/link/harness/onchain"
	"github.com/cosmos/ibc/link/harness/topology"
)

// waitPacketState polls the status API until the packet reports want, returning the matched
// PacketStatus. A status wait is an effect wait seen through the daemon (see wire.PacketStatus), so it
// runs on onchain.Await with the observed route end's profile. A packet seen in a different state is
// retained as the last probe error so a timeout names the state the row was stuck in.
//
// It takes the narrow onchain.StatusSource (which ibclink.Daemon satisfies) rather than the daemon
// handle: the wait needs nothing but status queries, and the seam keeps it independent of how many
// relayer processes serve them — and testable against a scripted source.
func waitPacketState(
	ctx context.Context,
	src onchain.StatusSource,
	packetID string,
	want wire.PacketState,
	prof topology.TimingProfile,
) (wire.PacketStatus, error) {
	desc := fmt.Sprintf("packet %s to report status %q", packetID, want)
	return onchain.Await(
		ctx,
		prof.CompletionBudget,
		prof.PollInterval,
		desc,
		func(ctx context.Context) (wire.PacketStatus, bool, error) {
			s, err := src.Status(ctx, wire.StatusQuery{PacketID: packetID})
			if err != nil {
				return wire.PacketStatus{}, false, err // transient; retried within the budget
			}
			ps, ok := s.Packet(packetID)
			if !ok {
				return wire.PacketStatus{}, false, fmt.Errorf("packet %s not present in daemon status", packetID)
			}
			if ps.State != want {
				// Not-yet progress, retained so a timeout names the state the row was stuck in.
				return wire.PacketStatus{}, false, fmt.Errorf("packet %s in state %q", packetID, ps.State)
			}
			return ps, true, nil
		},
	)
}

// waitPacketStable asserts the packet remains in want across prof's settle window — SettleObservations
// samples at prof.PollInterval, so the window is a property of the observed chain (it must outlast a block
// before a state is trusted stable), not a fixed observation count.
//
// This is the repo's one stability assertion — the condition must hold at EVERY sample, there is no
// "done early" — so it is not a wait-until and fits neither wait primitive (onchain.Await and poll.Until
// both stop at first success). It keeps its own bounded ticker loop by design; see AGENTS.md.
func waitPacketStable(
	ctx context.Context,
	src onchain.StatusSource,
	packetID string,
	want wire.PacketState,
	prof topology.TimingProfile,
) error {
	// Bound the whole assertion by the completion budget so a status endpoint that accepts but never
	// answers can't wedge it forever (unlike waitPacketState, this loop has no onchain.Await budget of its
	// own). Each Status call runs on this ctx, so a hung request also trips the deadline; the daemon's
	// http.Client carries its own ceiling too.
	ctx, cancel := context.WithTimeout(ctx, prof.CompletionBudget)
	defer cancel()

	ticker := time.NewTicker(prof.PollInterval)
	defer ticker.Stop()
	for i := 0; i < prof.SettleObservations(); i++ {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context canceled while watching packet %s stay %q: %w", packetID, want, ctx.Err())
		case <-ticker.C:
		}
		s, err := src.Status(ctx, wire.StatusQuery{PacketID: packetID})
		if err != nil {
			return err
		}
		ps, ok := s.Packet(packetID)
		if !ok {
			return fmt.Errorf("packet %s must stay present in the ledger", packetID)
		}
		if ps.State != want {
			return fmt.Errorf("packet %s must remain %q, got %q", packetID, want, ps.State)
		}
	}
	return nil
}
