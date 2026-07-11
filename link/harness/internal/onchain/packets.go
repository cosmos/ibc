// Package onchain contains family-independent assertions backed by per-chain Readers.
package onchain

import (
	"context"
	"fmt"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"
)

type Packets struct {
	readers map[string]Reader
}

func NewPackets(readers map[string]Reader) Packets {
	return Packets{readers: readers}
}

func (c Packets) TrackIFT(a *IFTAction) *IFTTracker {
	return &IFTTracker{packetTracker: packetTracker{corr: c, action: a}, ift: a}
}

func (c Packets) TrackGMP(a *GMPAction) *GMPTracker {
	return &GMPTracker{packetTracker: packetTracker{corr: c, action: a}, gmp: a}
}

type packetAction interface {
	ID() string
	packet() *PacketAction
}

type packetTracker struct {
	corr   Packets
	action packetAction
}

func (tr *packetTracker) packet() *PacketAction { return tr.action.packet() }

type IFTTracker struct {
	packetTracker
	ift *IFTAction
}

type GMPTracker struct {
	packetTracker
	gmp *GMPAction
}

func (tr *IFTTracker) VerifyComplete(ctx context.Context) error {
	packet := tr.packet()
	rdr, err := tr.reader(packet.Destination)
	if err != nil {
		return err
	}
	recv, err := rdr.AwaitIFTReceived(ctx, packet.RouteID, packet.Sequence)
	if err != nil {
		return fmt.Errorf("onchain: destination IFT Received for packet %s (seq %d) not observed on chain %s: %w",
			tr.action.ID(), packet.Sequence, packet.Destination, err)
	}
	// Both sides were canonicalized by the same Reader, so exact string compare is valid.
	if recv.Receiver != tr.ift.Receiver {
		return fmt.Errorf("onchain: IFT Received receiver for packet %s: got %s, want %s",
			tr.action.ID(), recv.Receiver, tr.ift.Receiver)
	}
	if recv.Amount.Cmp(tr.ift.Amount) != 0 {
		return fmt.Errorf("onchain: IFT Received amount for packet %s: got %s, want %s",
			tr.action.ID(), recv.Amount, tr.ift.Amount)
	}
	return nil
}

// Success=false is the error-ack outcome, not a delivery failure.
func (tr *GMPTracker) VerifyComplete(ctx context.Context) error {
	packet := tr.packet()
	rdr, err := tr.reader(packet.Destination)
	if err != nil {
		return err
	}
	recv, err := rdr.AwaitGMPReceived(ctx, packet.RouteID, packet.Sequence)
	if err != nil {
		return fmt.Errorf("onchain: destination GMP Received for packet %s (seq %d) not observed on chain %s: %w",
			tr.action.ID(), packet.Sequence, packet.Destination, err)
	}
	if recv.Target != tr.gmp.Target {
		return fmt.Errorf("onchain: GMP Received target for packet %s: got %s, want %s",
			tr.action.ID(), recv.Target, tr.gmp.Target)
	}
	if !recv.Success {
		return fmt.Errorf("onchain: GMP Received(seq=%d) success=false for packet %s: the target call reverted on %s",
			packet.Sequence, tr.action.ID(), packet.Destination)
	}
	return nil
}

// Success=false is the error-ack outcome, not a delivery failure.
func (tr *GMPTracker) VerifyErrorAck(ctx context.Context) error {
	packet := tr.packet()
	rdr, err := tr.reader(packet.Destination)
	if err != nil {
		return err
	}
	recv, err := rdr.AwaitGMPReceived(ctx, packet.RouteID, packet.Sequence)
	if err != nil {
		return fmt.Errorf("onchain: destination GMP Received for packet %s (seq %d) not observed on chain %s: %w",
			tr.action.ID(), packet.Sequence, packet.Destination, err)
	}
	if recv.Target != tr.gmp.Target {
		return fmt.Errorf("onchain: GMP Received target for packet %s: got %s, want %s",
			tr.action.ID(), recv.Target, tr.gmp.Target)
	}
	if recv.Success {
		return fmt.Errorf(
			"onchain: GMP Received(seq=%d) success=true for packet %s: "+
				"the target call did NOT revert, so this is not an error-ack",
			packet.Sequence,
			tr.action.ID(),
		)
	}
	return nil
}

func (tr *IFTTracker) VerifyTimedOut(ctx context.Context) error {
	packet := tr.packet()
	rdr, err := tr.reader(packet.Source)
	if err != nil {
		return err
	}
	ev, err := rdr.AwaitIFTRefunded(ctx, packet.Sequence)
	if err != nil {
		return fmt.Errorf("onchain: source IFT Refunded for packet %s (seq %d) not observed on chain %s: %w",
			tr.action.ID(), packet.Sequence, packet.Source, err)
	}
	if ev.Amount.Cmp(tr.ift.Amount) != 0 {
		return fmt.Errorf("onchain: IFT Refunded amount for packet %s: got %s, want %s",
			tr.action.ID(), ev.Amount, tr.ift.Amount)
	}
	return nil
}

type StatusSource interface {
	Status(ctx context.Context, q wire.StatusQuery) (*wire.Status, error)
}

func (tr *packetTracker) StatusCrossCheck(ctx context.Context, d StatusSource) error {
	packet := tr.packet()
	dstRdr, err := tr.reader(packet.Destination)
	if err != nil {
		return err
	}
	budget := dstRdr.Budget()

	desc := fmt.Sprintf("status cross-check: packet %s complete in daemon status", tr.action.ID())
	_, err = Await(ctx, budget.StatusRow, budget.Poll, desc, func(ctx context.Context) (struct{}, bool, error) {
		status, statusErr := d.Status(ctx, wire.StatusQuery{PacketID: tr.action.ID()})
		if statusErr != nil {
			return struct{}{}, false, statusErr
		}
		ps, ok := status.Packet(tr.action.ID())
		if !ok {
			return struct{}{}, false, fmt.Errorf("packet %s not present in daemon status", tr.action.ID())
		}
		if ps.State != wire.PacketComplete || ps.EffectTxHash == "" {
			return struct{}{}, false, fmt.Errorf(
				"packet %s not complete (state %q, recvTx %q)",
				tr.action.ID(),
				ps.State,
				ps.EffectTxHash,
			)
		}
		if ps.RouteID != packet.RouteID {
			return struct{}{}, true, fmt.Errorf("status cross-check: packet %s route %q != action route %q",
				tr.action.ID(), ps.RouteID, packet.RouteID)
		}
		if ps.Sequence != packet.Sequence {
			return struct{}{}, true, fmt.Errorf("status cross-check: packet %s sequence %d != action sequence %d",
				tr.action.ID(), ps.Sequence, packet.Sequence)
		}
		return struct{}{}, true, nil
	})
	return err
}

func (tr *packetTracker) reader(chainID string) (Reader, error) {
	return reader(tr.corr.readers, chainID)
}
