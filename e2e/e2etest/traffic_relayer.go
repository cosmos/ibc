package e2etest

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/ibclink"

	relayerv2 "github.com/cosmos/ibc/link/api/v2/relayer"
)

func AwaitState(
	ctx context.Context,
	relayer *ibclink.Relayer,
	packet Packet,
	want relayerv2.PacketState,
	timing environment.Timing,
) error {
	if relayer == nil {
		return errors.New("e2etest: relayer is required")
	}
	packetID := packetID(packet)

	description := fmt.Sprintf("packet %s to report status %q", packetID, want)
	_, err := await(
		ctx,
		timing.CompletionBudget,
		timing.PollInterval,
		description,
		func(ctx context.Context) (*relayerv2.PacketStatus, bool, error) {
			observed, state, ok, err := observeStatus(ctx, relayer, packet)
			if err != nil {
				return nil, false, err
			}
			if !ok {
				return nil, false, fmt.Errorf("packet %s is absent from relayer status", packetID)
			}
			if state != want {
				return nil, false, fmt.Errorf(
					"packet %s is %q, want %q",
					packetID,
					state,
					want,
				)
			}
			if err := verifySourceTx(packetID, packet, observed); err != nil {
				return nil, true, err
			}
			if err := validateTerminalStatus(packetID, state, observed); err != nil {
				return nil, true, err
			}
			return observed, true, nil
		},
	)
	return err
}

// AwaitStable requires the packet to remain in one state across the Chain's
// settle window after that state is first observed.
func AwaitStable(
	ctx context.Context,
	relayer *ibclink.Relayer,
	packet Packet,
	want relayerv2.PacketState,
	timing environment.Timing,
) error {
	ctx, cancel := context.WithTimeout(ctx, timing.CompletionBudget)
	defer cancel()
	if err := AwaitState(ctx, relayer, packet, want, timing); err != nil {
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
		observed, state, ok, err := observeStatus(ctx, relayer, packet)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("packet %s must stay present in relayer status", packetID)
		}
		if state != want {
			return fmt.Errorf("packet %s must remain %q, got %q", packetID, want, state)
		}
		if err := verifySourceTx(packetID, packet, observed); err != nil {
			return err
		}
	}
	return nil
}

// Relay submits the packet's source transaction for relaying and confirms the
// relayer enumerated the packet.
func Relay(ctx context.Context, relayer *ibclink.Relayer, packet Packet) error {
	if relayer == nil {
		return errors.New("e2etest: relayer is required")
	}
	if err := relayer.Relay(ctx, string(packet.Source), packet.SourceTxHash); err != nil {
		return err
	}
	statuses, err := relayer.PacketStatuses(ctx, string(packet.Source), packet.SourceTxHash)
	if err != nil {
		return err
	}
	if statusForSequence(statuses, packet.Sequence) == nil {
		return fmt.Errorf(
			"e2etest: relay result for packet %s did not include it: %v",
			packetID(packet),
			statuses,
		)
	}
	return nil
}

// observeStatus reports the relayer's wire status for the packet. A nil
// status with ok set means the relayer has no record of the source
// transaction yet: the packet reads as pending; on non-manual routes it is
// submitted for relaying first, standing in for the relayer's on-chain
// packet discovery until the product grows one.
func observeStatus(
	ctx context.Context,
	relayer *ibclink.Relayer,
	packet Packet,
) (*relayerv2.PacketStatus, relayerv2.PacketState, bool, error) {
	statuses, err := relayer.PacketStatuses(ctx, string(packet.Source), packet.SourceTxHash)
	if ibclink.IsStatusNotFound(err) {
		if !relayer.ManualRoute(string(packet.RouteID)) {
			if relayErr := relayer.Relay(ctx, string(packet.Source), packet.SourceTxHash); relayErr != nil {
				return nil, 0, false, relayErr
			}
		}
		return nil, relayerv2.PacketState_PACKET_STATE_PENDING, true, nil
	}
	if err != nil {
		return nil, 0, false, err
	}
	observed := statusForSequence(statuses, packet.Sequence)
	if observed == nil {
		return nil, 0, false, nil
	}
	return observed, observed.GetState(), true, nil
}

func statusForSequence(statuses []*relayerv2.PacketStatus, sequence uint64) *relayerv2.PacketStatus {
	for _, status := range statuses {
		if status.GetSequenceNumber() == sequence {
			return status
		}
	}
	return nil
}

func packetID(packet Packet) string {
	return fmt.Sprintf("%s-%d", packet.RouteID, packet.Sequence)
}

func verifySourceTx(packetID string, packet Packet, observed *relayerv2.PacketStatus) error {
	if observed == nil {
		return nil
	}
	sendTx := observed.GetSendTx().GetTxHash()
	if sendTx != packet.SourceTxHash {
		return fmt.Errorf(
			"e2etest: packet %s status source transaction %q, want %q",
			packetID,
			sendTx,
			packet.SourceTxHash,
		)
	}
	return nil
}

func validateTerminalStatus(
	packetID string,
	state relayerv2.PacketState,
	observed *relayerv2.PacketStatus,
) error {
	switch state {
	case relayerv2.PacketState_PACKET_STATE_TIMED_OUT:
		if observed.GetTimeoutTx().GetTxHash() == "" {
			return fmt.Errorf("e2etest: %s packet %s has no timeout transaction", state, packetID)
		}
	case relayerv2.PacketState_PACKET_STATE_SUCCEEDED,
		relayerv2.PacketState_PACKET_STATE_REJECTED:
		if observed.GetRecvTx().GetTxHash() == "" {
			return fmt.Errorf("e2etest: %s packet %s has no receive transaction", state, packetID)
		}
		if observed.GetAckTx().GetTxHash() == "" {
			return fmt.Errorf("e2etest: %s packet %s has no acknowledgement transaction", state, packetID)
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
