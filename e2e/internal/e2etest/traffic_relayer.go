// SPDX-License-Identifier: Apache-2.0

package e2etest

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cosmos/ibc/e2e/internal/harness/ibclink"
	relayerv2 "github.com/cosmos/ibc/link/api/v2/relayer"
)

// AwaitState waits for the packet to reach want. If want is pending and the
// relayer has not indexed the source transaction, it returns (nil, nil).
func AwaitState(
	ctx context.Context,
	relayer *ibclink.Relayer,
	packet PacketTx,
	want relayerv2.PacketState,
) (*relayerv2.PacketStatus, error) {
	if relayer == nil {
		return nil, errors.New("e2etest: relayer is required")
	}
	policy, err := packetWaitPolicy(relayer, packet)
	if err != nil {
		return nil, err
	}
	return awaitPacketState(
		ctx,
		packet,
		want,
		policy,
		func(ctx context.Context) (*relayerv2.PacketStatus, relayerv2.PacketState, bool, error) {
			return observeStatus(ctx, relayer, packet)
		},
	)
}

type packetStatusObserver func(context.Context) (*relayerv2.PacketStatus, relayerv2.PacketState, bool, error)

func awaitPacketState(
	ctx context.Context,
	packet PacketTx,
	want relayerv2.PacketState,
	policy ibclink.WaitPolicy,
	observe packetStatusObserver,
) (*relayerv2.PacketStatus, error) {
	description := fmt.Sprintf("packet %s to report status %q", packet, want)
	return await(
		ctx,
		policy.CompletionBudget,
		policy.StatusPoll,
		description,
		func(ctx context.Context) (*relayerv2.PacketStatus, bool, error) {
			observed, state, ok, err := observe(ctx)
			if err != nil {
				return nil, false, err
			}
			if !ok {
				return nil, false, fmt.Errorf("packet %s is absent from relayer status", packet)
			}
			if state != want {
				return nil, false, fmt.Errorf(
					"packet %s is %q, want %q",
					packet,
					state,
					want,
				)
			}
			if err := verifySourceTx(packet, observed); err != nil {
				return nil, true, err
			}
			if err := validateTerminalStatus(packet, state, observed); err != nil {
				return nil, true, err
			}
			return observed, true, nil
		},
	)
}

// AwaitStable requires the packet to remain in one state across its route's
// stability window after that state is first observed.
func AwaitStable(
	ctx context.Context,
	relayer *ibclink.Relayer,
	packet PacketTx,
	want relayerv2.PacketState,
) error {
	if relayer == nil {
		return errors.New("e2etest: relayer is required")
	}
	policy, err := packetWaitPolicy(relayer, packet)
	if err != nil {
		return err
	}
	observe := func(ctx context.Context) (*relayerv2.PacketStatus, relayerv2.PacketState, bool, error) {
		return observeStatus(ctx, relayer, packet)
	}
	return awaitStablePacketState(ctx, packet, want, policy, observe)
}

func awaitStablePacketState(
	ctx context.Context,
	packet PacketTx,
	want relayerv2.PacketState,
	policy ibclink.WaitPolicy,
	observe packetStatusObserver,
) error {
	if _, err := awaitPacketState(ctx, packet, want, policy, observe); err != nil {
		return err
	}
	return watchPacketState(ctx, packet, want, policy, observe)
}

func watchPacketState(
	ctx context.Context,
	packet PacketTx,
	want relayerv2.PacketState,
	policy ibclink.WaitPolicy,
	observe packetStatusObserver,
) error {
	check := func() error {
		observed, state, ok, err := observe(ctx)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("packet %s must stay present in relayer status", packet)
		}
		if state != want {
			return fmt.Errorf("packet %s must remain %q, got %q", packet, want, state)
		}
		if err := verifySourceTx(packet, observed); err != nil {
			return err
		}
		return validateTerminalStatus(packet, state, observed)
	}
	cancellationError := func() error {
		return fmt.Errorf(
			"context canceled while watching packet %s stay %q: %w",
			packet,
			want,
			ctx.Err(),
		)
	}
	ticker := time.NewTicker(policy.StatusPoll)
	defer ticker.Stop()
	timer := time.NewTimer(policy.StabilityWindow)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return cancellationError()
		case <-timer.C:
			if ctx.Err() != nil {
				return cancellationError()
			}
			if err := check(); err != nil {
				return err
			}
			if ctx.Err() != nil {
				return cancellationError()
			}
			return nil
		case <-ticker.C:
		}
		if err := check(); err != nil {
			return err
		}
	}
}

func packetWaitPolicy(relayer *ibclink.Relayer, packet PacketTx) (ibclink.WaitPolicy, error) {
	if packet.RouteID == "" {
		return ibclink.WaitPolicy{}, fmt.Errorf("e2etest: packet sequence %d has no route id", packet.Sequence)
	}
	policy, ok := relayer.WaitPolicy(string(packet.RouteID))
	if !ok {
		return ibclink.WaitPolicy{}, fmt.Errorf(
			"e2etest: packet %s has no wait policy for route %q",
			packet,
			packet.RouteID,
		)
	}
	return policy, nil
}

// RelayAll submits every packet in the packet's source transaction and
// confirms the relayer enumerated this packet.
func RelayAll(ctx context.Context, relayer *ibclink.Relayer, packet PacketTx) error {
	if relayer == nil {
		return errors.New("e2etest: relayer is required")
	}
	if err := relayer.RelayAll(ctx, string(packet.Source), packet.SourceTxHash); err != nil {
		return err
	}
	statuses, err := relayer.PacketStatuses(ctx, string(packet.Source), packet.SourceTxHash)
	if err != nil {
		return err
	}
	if statusForPacket(statuses, packet) == nil {
		return fmt.Errorf(
			"e2etest: relay result for packet %s did not include it: %v",
			packet,
			statuses,
		)
	}
	return nil
}

// RelaySelected submits explicit packets from one source transaction.
func RelaySelected(ctx context.Context, relayer *ibclink.Relayer, packets ...PacketTx) error {
	if relayer == nil {
		return errors.New("e2etest: relayer is required")
	}
	if len(packets) == 0 {
		return errors.New("e2etest: selected packets are required")
	}

	source := packets[0]
	selectors := make([]*relayerv2.PacketSelector, len(packets))
	for i, packet := range packets {
		if packet.Source != source.Source || packet.SourceTxHash != source.SourceTxHash {
			return errors.New("e2etest: selected packets must share a source transaction")
		}
		selectors[i] = &relayerv2.PacketSelector{
			SourceClientId: packet.SourceClientID,
			SequenceNumber: packet.Sequence,
		}
	}
	return relayer.RelaySelected(ctx, string(source.Source), source.SourceTxHash, selectors...)
}

// observeStatus reports the relayer's wire status for the packet. A nil
// status with ok set means the relayer has no record of the source
// transaction yet: the packet reads as pending; on non-manual routes it is
// submitted for relaying first, standing in for the relayer's on-chain
// packet discovery until the product grows one.
func observeStatus(
	ctx context.Context,
	relayer *ibclink.Relayer,
	packet PacketTx,
) (*relayerv2.PacketStatus, relayerv2.PacketState, bool, error) {
	statuses, err := relayer.PacketStatuses(ctx, string(packet.Source), packet.SourceTxHash)
	if err != nil {
		return nil, 0, false, err
	}
	// An empty listing means the relayer has not indexed the transaction, which
	// the Status endpoint used to report as a NotFound error.
	if len(statuses) == 0 {
		if !relayer.ManualRoute(string(packet.RouteID)) {
			if relayErr := relayer.RelayAll(ctx, string(packet.Source), packet.SourceTxHash); relayErr != nil {
				return nil, 0, false, relayErr
			}
		}
		return nil, relayerv2.PacketState_PACKET_STATE_PENDING, true, nil
	}
	observed := statusForPacket(statuses, packet)
	if observed == nil {
		return nil, 0, false, nil
	}
	return observed, observed.GetState(), true, nil
}

func statusForPacket(statuses []*relayerv2.PacketStatus, packet PacketTx) *relayerv2.PacketStatus {
	for _, status := range statuses {
		if status.GetSourceClientId() == packet.SourceClientID && status.GetSequenceNumber() == packet.Sequence {
			return status
		}
	}
	return nil
}

func verifySourceTx(packet PacketTx, observed *relayerv2.PacketStatus) error {
	if observed == nil {
		return nil
	}
	sendTx := observed.GetSendTx().GetTxHash()
	if sendTx != packet.SourceTxHash {
		return fmt.Errorf(
			"e2etest: packet %s status source transaction %q, want %q",
			packet,
			sendTx,
			packet.SourceTxHash,
		)
	}
	return nil
}

func validateTerminalStatus(
	packet PacketTx,
	state relayerv2.PacketState,
	observed *relayerv2.PacketStatus,
) error {
	switch state {
	case relayerv2.PacketState_PACKET_STATE_TIMED_OUT:
		if observed.GetTimeoutTx().GetTxHash() == "" {
			return fmt.Errorf("e2etest: %s packet %s has no timeout transaction", state, packet)
		}
		if observed.GetRecvTx().GetTxHash() != "" {
			return fmt.Errorf("e2etest: %s packet %s has a receive transaction", state, packet)
		}
		if observed.GetAckTx().GetTxHash() != "" {
			return fmt.Errorf("e2etest: %s packet %s has an acknowledgement transaction", state, packet)
		}
	case relayerv2.PacketState_PACKET_STATE_SUCCEEDED,
		relayerv2.PacketState_PACKET_STATE_REJECTED:
		if observed.GetRecvTx().GetTxHash() == "" {
			return fmt.Errorf("e2etest: %s packet %s has no receive transaction", state, packet)
		}
		if observed.GetAckTx().GetTxHash() == "" {
			return fmt.Errorf("e2etest: %s packet %s has no acknowledgement transaction", state, packet)
		}
	}
	return nil
}
