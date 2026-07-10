package harness

import (
	"context"
	"fmt"
	"time"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"
	"github.com/cosmos/ibc/link/harness/internal/onchain"
	"github.com/cosmos/ibc/link/harness/topology"
)

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
				return wire.PacketStatus{}, false, err
			}
			ps, ok := s.Packet(packetID)
			if !ok {
				return wire.PacketStatus{}, false, fmt.Errorf("packet %s not present in daemon status", packetID)
			}
			if ps.State != want {
				return wire.PacketStatus{}, false, fmt.Errorf("packet %s in state %q", packetID, ps.State)
			}
			return ps, true, nil
		},
	)
}

// Stability assertion: the condition must hold at every sample (no early exit), so this fits
// neither onchain.Await nor poll.Until; see AGENTS.md.
func waitPacketStable(
	ctx context.Context,
	src onchain.StatusSource,
	packetID string,
	want wire.PacketState,
	prof topology.TimingProfile,
) error {
	// Bound the whole loop by the completion budget so a hung status endpoint cannot wedge it.
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
