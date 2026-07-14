package synthetic

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/cosmos/ibc/link/e2e/internal/observe"
	"github.com/cosmos/ibc/link/e2e/internal/testapp"
	"github.com/cosmos/ibc/link/harness/environment"
	"github.com/cosmos/ibc/link/harness/ibclink"
	"github.com/cosmos/ibc/link/harness/ibclink/wire"
)

func AwaitState(
	ctx context.Context,
	relayer *ibclink.Relayer,
	packet testapp.Packet,
	want wire.PacketState,
	timing environment.Timing,
) (wire.PacketStatus, error) {
	if relayer == nil {
		return wire.PacketStatus{}, errors.New("synthetic: relayer is required")
	}
	packetID, err := packetID(packet)
	if err != nil {
		return wire.PacketStatus{}, err
	}

	description := fmt.Sprintf("packet %s to report status %q", packetID, want)
	return observe.Await(
		ctx,
		timing.CompletionBudget,
		timing.PollInterval,
		description,
		func(ctx context.Context) (wire.PacketStatus, bool, error) {
			status, err := relayer.Status(ctx, wire.StatusQuery{PacketID: packetID})
			if err != nil {
				return wire.PacketStatus{}, false, err
			}
			observed, ok := status.Packet(packetID)
			if !ok {
				return wire.PacketStatus{}, false, fmt.Errorf("packet %s is absent from relayer status", packetID)
			}
			if observed.State != want {
				return wire.PacketStatus{}, false, fmt.Errorf(
					"packet %s is %q, want %q",
					packetID,
					observed.State,
					want,
				)
			}
			if err := crossCheck(packet, observed); err != nil {
				return wire.PacketStatus{}, true, err
			}
			if err := validateTerminalStatus(observed); err != nil {
				return wire.PacketStatus{}, true, err
			}
			return observed, true, nil
		},
	)
}

// AwaitStable requires the packet to remain in one state across the Chain's
// settle window after that state is first observed.
func AwaitStable(
	ctx context.Context,
	relayer *ibclink.Relayer,
	packet testapp.Packet,
	want wire.PacketState,
	timing environment.Timing,
) error {
	ctx, cancel := context.WithTimeout(ctx, timing.CompletionBudget)
	defer cancel()
	if _, err := AwaitState(ctx, relayer, packet, want, timing); err != nil {
		return err
	}

	packetID, err := packetID(packet)
	if err != nil {
		return err
	}
	ticker := time.NewTicker(timing.PollInterval)
	defer ticker.Stop()
	for range settleObservations(timing) {
		select {
		case <-ctx.Done():
			return fmt.Errorf(
				"context canceled while watching packet %s stay %q: %w",
				packetID,
				want,
				ctx.Err(),
			)
		case <-ticker.C:
		}
		status, err := relayer.Status(ctx, wire.StatusQuery{PacketID: packetID})
		if err != nil {
			return err
		}
		observed, ok := status.Packet(packetID)
		if !ok {
			return fmt.Errorf("packet %s must stay present in relayer status", packetID)
		}
		if observed.State != want {
			return fmt.Errorf("packet %s must remain %q, got %q", packetID, want, observed.State)
		}
		if err := crossCheck(packet, observed); err != nil {
			return err
		}
	}
	return nil
}

func Relay(ctx context.Context, relayer *ibclink.Relayer, packet testapp.Packet) error {
	if relayer == nil {
		return errors.New("synthetic: relayer is required")
	}
	packetID, err := packetID(packet)
	if err != nil {
		return err
	}
	result, err := relayer.Relay(ctx, wire.RelayRequest{
		SourceChainID: string(packet.Source),
		SourceTxHash:  packet.SourceTxHash,
	})
	if err != nil {
		return err
	}
	if !slices.Contains(result.PacketIDs, packetID) {
		return fmt.Errorf(
			"synthetic: relay result for packet %s did not include it: %v",
			packetID,
			result.PacketIDs,
		)
	}
	return nil
}

func packetID(packet testapp.Packet) (string, error) {
	var application wire.AppType
	switch packet.Application() {
	case testapp.ApplicationIFT:
		application = wire.AppTypeIFT
	case testapp.ApplicationGMP:
		application = wire.AppTypeGMP
	default:
		return "", fmt.Errorf("synthetic: unsupported test application %q", packet.Application())
	}
	return wire.PacketID(string(packet.RouteID), application, packet.Sequence), nil
}

func crossCheck(packet testapp.Packet, observed wire.PacketStatus) error {
	packetID, err := packetID(packet)
	if err != nil {
		return err
	}
	if observed.PacketID != packetID {
		return fmt.Errorf("synthetic: status packet id %q, want %q", observed.PacketID, packetID)
	}
	if observed.RouteID != string(packet.RouteID) {
		return fmt.Errorf(
			"synthetic: packet %s status route %q, want %q",
			packetID,
			observed.RouteID,
			packet.RouteID,
		)
	}
	if observed.Sequence != packet.Sequence {
		return fmt.Errorf(
			"synthetic: packet %s status sequence %d, want %d",
			packetID,
			observed.Sequence,
			packet.Sequence,
		)
	}
	if observed.SourceTxHash != packet.SourceTxHash {
		return fmt.Errorf(
			"synthetic: packet %s status source transaction %q, want %q",
			packetID,
			observed.SourceTxHash,
			packet.SourceTxHash,
		)
	}
	return nil
}

func validateTerminalStatus(status wire.PacketStatus) error {
	switch status.State {
	case wire.PacketComplete:
		if status.EffectTxHash == "" {
			return fmt.Errorf("synthetic: complete packet %s has no effect transaction", status.PacketID)
		}
	case wire.PacketTimedOut, wire.PacketErrorAck:
		if status.EffectTxHash == "" {
			return fmt.Errorf("synthetic: %s packet %s has no effect transaction", status.State, status.PacketID)
		}
		if status.Reason == "" {
			return fmt.Errorf("synthetic: %s packet %s has no reason", status.State, status.PacketID)
		}
	}
	return nil
}

func settleObservations(timing environment.Timing) int {
	count := int((timing.SettleWindow + timing.PollInterval - 1) / timing.PollInterval)
	if count < 1 {
		return 1
	}
	return count
}
