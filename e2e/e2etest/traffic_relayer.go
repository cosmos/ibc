package e2etest

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/ibclink"
	"github.com/cosmos/ibc/link/cmd/relayercmd"

	relayerv2 "github.com/cosmos/ibc/link/api/v2/relayer"
)

// PacketFailed is the relayer-reported terminal failure state. No test wants
// it; it exists so mismatch errors read well.
const PacketFailed relayercmd.PacketState = "failed"

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
			observed, ok, err := observeStatus(ctx, relayer, packet)
			if err != nil {
				return relayercmd.PacketStatus{}, false, err
			}
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
		observed, ok, err := observeStatus(ctx, relayer, packet)
		if err != nil {
			return err
		}
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

// Relay submits the packet's source transaction for relaying and confirms the
// relayer enumerated the packet.
func Relay(ctx context.Context, relayer *ibclink.Relayer, packet Packet) error {
	if relayer == nil {
		return errors.New("e2etest: relayer is required")
	}
	if err := relayer.Relay(ctx, relayercmd.RelayRequest{
		SourceChainID: string(packet.Source),
		SourceTxHash:  packet.SourceTxHash,
	}); err != nil {
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

// observeStatus maps the relayer's packet status to the acceptance-test
// vocabulary. A source transaction the relayer does not know yet reads as
// pending; on non-manual routes it is submitted for relaying first, standing
// in for the relayer's on-chain packet discovery until the product grows one.
func observeStatus(
	ctx context.Context,
	relayer *ibclink.Relayer,
	packet Packet,
) (relayercmd.PacketStatus, bool, error) {
	statuses, err := relayer.PacketStatuses(ctx, string(packet.Source), packet.SourceTxHash)
	if ibclink.IsStatusNotFound(err) {
		if !relayer.ManualRoute(string(packet.RouteID)) {
			if relayErr := relayer.Relay(ctx, relayercmd.RelayRequest{
				SourceChainID: string(packet.Source),
				SourceTxHash:  packet.SourceTxHash,
			}); relayErr != nil {
				return relayercmd.PacketStatus{}, false, relayErr
			}
		}
		return pendingStatus(packet), true, nil
	}
	if err != nil {
		return relayercmd.PacketStatus{}, false, err
	}
	observed := statusForSequence(statuses, packet.Sequence)
	if observed == nil {
		return relayercmd.PacketStatus{}, false, nil
	}
	return mapStatus(packet, observed), true, nil
}

func statusForSequence(statuses []*relayerv2.PacketStatus, sequence uint64) *relayerv2.PacketStatus {
	for _, status := range statuses {
		if status.GetSequenceNumber() == sequence {
			return status
		}
	}
	return nil
}

// pendingStatus reads a source transaction the relayer has no record of as a
// pending packet: nothing has happened to it yet.
func pendingStatus(packet Packet) relayercmd.PacketStatus {
	return relayercmd.PacketStatus{
		PacketID:     packetID(packet),
		RouteID:      string(packet.RouteID),
		Sequence:     packet.Sequence,
		State:        relayercmd.PacketPending,
		SourceTxHash: packet.SourceTxHash,
	}
}

func mapStatus(packet Packet, observed *relayerv2.PacketStatus) relayercmd.PacketStatus {
	status := pendingStatus(packet)
	status.Sequence = observed.GetSequenceNumber()
	if sendTx := observed.GetSendTx().GetTxHash(); sendTx != "" {
		status.SourceTxHash = sendTx
	}
	switch observed.GetState() {
	case relayerv2.TransferState_TRANSFER_STATE_COMPLETE:
		switch {
		case observed.GetTimeoutTx() != nil:
			status.State = relayercmd.PacketTimedOut
			status.EffectTxHash = observed.GetTimeoutTx().GetTxHash()
		case observed.GetWriteAckError():
			status.State = relayercmd.PacketErrorAck
			status.EffectTxHash = observed.GetRecvTx().GetTxHash()
		default:
			status.State = relayercmd.PacketComplete
			status.EffectTxHash = observed.GetRecvTx().GetTxHash()
		}
	case relayerv2.TransferState_TRANSFER_STATE_FAILED:
		status.State = PacketFailed
	default:
	}
	return status
}

func packetID(packet Packet) string {
	return fmt.Sprintf("%s-%d", packet.RouteID, packet.Sequence)
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
	case relayercmd.PacketComplete, relayercmd.PacketTimedOut, relayercmd.PacketErrorAck:
		if status.EffectTxHash == "" {
			return fmt.Errorf("e2etest: %s packet %s has no effect transaction", status.State, status.PacketID)
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
