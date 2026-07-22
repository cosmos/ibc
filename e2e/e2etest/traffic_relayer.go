package e2etest

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/ibclink"
	"github.com/cosmos/ibc/link/cmd/relayercmd"
)

func AwaitState(
	ctx context.Context,
	relayer *ibclink.Relayer,
	packet Packet,
	want relayercmd.PacketState,
	timing environment.Timing,
) (relayercmd.PacketStatus, error) {
	if relayer == nil {
		return relayercmd.PacketStatus{}, errors.New("e2etest: relayer is required")
	}
	packetID := packetID(packet)

	description := fmt.Sprintf("packet %s to report status %q", packetID, want)
	return await(
		ctx,
		timing.CompletionBudget,
		timing.PollInterval,
		description,
		func(ctx context.Context) (relayercmd.PacketStatus, bool, error) {
			status, err := relayer.Status(ctx, relayercmd.StatusQuery{PacketID: packetID})
			if err != nil {
				return relayercmd.PacketStatus{}, false, err
			}
			observed, ok := status.Packet(packetID)
			if !ok {
				return relayercmd.PacketStatus{}, false, fmt.Errorf("packet %s is absent from relayer status", packetID)
			}
			if observed.State != want {
				return relayercmd.PacketStatus{}, false, fmt.Errorf(
					"packet %s is %q, want %q",
					packetID,
					observed.State,
					want,
				)
			}
			if err := crossCheck(packetID, packet, observed); err != nil {
				return relayercmd.PacketStatus{}, true, err
			}
			if err := validateTerminalStatus(observed); err != nil {
				return relayercmd.PacketStatus{}, true, err
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
	packet Packet,
	want relayercmd.PacketState,
	timing environment.Timing,
) error {
	ctx, cancel := context.WithTimeout(ctx, timing.CompletionBudget)
	defer cancel()
	if _, err := AwaitState(ctx, relayer, packet, want, timing); err != nil {
		return err
	}

	packetID := packetID(packet)
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
		status, err := relayer.Status(ctx, relayercmd.StatusQuery{PacketID: packetID})
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
		if err := crossCheck(packetID, packet, observed); err != nil {
			return err
		}
	}
	return nil
}

func Relay(ctx context.Context, relayer *ibclink.Relayer, packet Packet) error {
	if relayer == nil {
		return errors.New("e2etest: relayer is required")
	}
	packetID := packetID(packet)
	result, err := relayer.Relay(ctx, relayercmd.RelayRequest{
		SourceChainID: string(packet.Source),
		SourceTxHash:  packet.SourceTxHash,
	})
	if err != nil {
		return err
	}
	if !slices.Contains(result.PacketIDs, packetID) {
		return fmt.Errorf(
			"e2etest: relay result for packet %s did not include it: %v",
			packetID,
			result.PacketIDs,
		)
	}
	return nil
}

func packetID(packet Packet) string {
	return relayercmd.RoutePacketID(string(packet.RouteID), packet.Sequence)
}

func crossCheck(packetID string, packet Packet, observed relayercmd.PacketStatus) error {
	if observed.RouteID != string(packet.RouteID) {
		return fmt.Errorf(
			"e2etest: packet %s status route %q, want %q",
			packetID,
			observed.RouteID,
			packet.RouteID,
		)
	}
	if observed.Sequence != packet.Sequence {
		return fmt.Errorf(
			"e2etest: packet %s status sequence %d, want %d",
			packetID,
			observed.Sequence,
			packet.Sequence,
		)
	}
	if observed.SourceTxHash != packet.SourceTxHash {
		return fmt.Errorf(
			"e2etest: packet %s status source transaction %q, want %q",
			packetID,
			observed.SourceTxHash,
			packet.SourceTxHash,
		)
	}
	return nil
}

func validateTerminalStatus(status relayercmd.PacketStatus) error {
	switch status.State {
	case relayercmd.PacketComplete:
		if status.EffectTxHash == "" {
			return fmt.Errorf("e2etest: complete packet %s has no effect transaction", status.PacketID)
		}
	case relayercmd.PacketTimedOut, relayercmd.PacketErrorAck:
		if status.EffectTxHash == "" {
			return fmt.Errorf("e2etest: %s packet %s has no effect transaction", status.State, status.PacketID)
		}
		if status.Reason == "" {
			return fmt.Errorf("e2etest: %s packet %s has no reason", status.State, status.PacketID)
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
