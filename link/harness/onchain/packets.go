// Package onchain contains family-independent assertions backed by per-chain Readers.
package onchain

import (
	"context"
	"fmt"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"
)

// Packets is bound to one environment's per-chain Readers (keyed by Chain.ID). It is cheap to copy.
type Packets struct {
	readers map[string]Reader
}

// NewPackets builds a Packets over the env's per-chain Readers (keyed by Chain.ID).
func NewPackets(readers map[string]Reader) Packets {
	return Packets{readers: readers}
}

// TrackIFT begins correlating an IFT action across the packet lifecycle.
func (c Packets) TrackIFT(a *IFTAction) *IFTTracker {
	return &IFTTracker{packetTracker: packetTracker{corr: c, action: a}, ift: a}
}

// TrackGMP begins correlating a GMP action across the packet lifecycle.
func (c Packets) TrackGMP(a *GMPAction) *GMPTracker {
	return &GMPTracker{packetTracker: packetTracker{corr: c, action: a}, gmp: a}
}

type packetAction interface {
	ID() string
	packet() *PacketAction
}

// packetTracker is the app-agnostic correlation core the two typed trackers share: the bound readers,
// the packet coordinates, reader resolution, and the daemon status cross-check. The app-specific
// terminal floors live on IFTTracker/GMPTracker, so an app-mismatched assertion (an error-ack check on
// an IFT packet, a refund check on a GMP one) is a compile error, not a runtime guard.
type packetTracker struct {
	corr   Packets
	action packetAction
}

func (tr *packetTracker) packet() *PacketAction { return tr.action.packet() }

// IFTTracker correlates a single tracked IFT action.
type IFTTracker struct {
	packetTracker
	ift *IFTAction
}

// GMPTracker correlates a single tracked GMP action.
type GMPTracker struct {
	packetTracker
	gmp *GMPAction
}

// VerifyComplete is the IFT terminal floor: it waits for the destination IFT Received effect for the
// packet's sequence — the relayer's real on-chain mint — then asserts the mint credited the right amount
// to the right receiver the action submitted (the wrong amount/receiver fails here, not slips through),
// proving the user-visible outcome independent of the stub and of any balance check the suite may also run.
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
	// The matched event already carries the action's sequence (the wait keyed on it); also assert the
	// exact receiver and amount the action claimed — the wrong amount or wrong receiver must fail here,
	// not slip through. Both strings are the family's canonical form (Prepare canonicalized the action's
	// receiver through this Reader; the Reader normalizes the event the same way), so a value compare is
	// an exact match.
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

// VerifyComplete is the GMP terminal floor: it waits (bounded) for the destination GMP Received effect
// (seq, target, success=true), read via the harness's own Reader, not the stub. It asserts the matched
// effect reports the action's target and a successful target execution, so the floor proves delivery to the
// right target by itself: without the target check, a sequence+success match alone could pass a
// misdelivery. The exactly-once magnitude is still the GMP asserter's VerifyTargetChangedOnce job — it
// holds the pre-action state snapshot Packets lacks.
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
	// The matched event already carries the action's sequence (the wait keyed on it). Assert it delivered
	// to the action's target and that the target execution succeeded: the fixture records failed target
	// execution as success=false without reverting the delivery, so success=false is the error-ack outcome,
	// not a complete one — this happy-path floor requires success=true. The error-ack case is asserted
	// separately by VerifyErrorAck (success=false), which the relayer marks error_ack.
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

// VerifyErrorAck is the GMP error-ack terminal floor: it waits (bounded) for the destination GMP Received
// effect (seq, success=false) — the relayer did deliver but target execution failed, so ibc link returns
// an error acknowledgement rather than a success. Read via the harness's own Reader, not the stub. It
// asserts the delivery failed at the target, proving the error-ack outcome on-chain independent of the
// stub. The target is deliberately NOT asserted here: an error ack moves nothing, so some families (cosmos)
// have no delivery effect to read the target back from — the target is proven on the success path
// (VerifyComplete) instead. The error-ack leg is GMP-only (IFT's receiveTransfer always succeeds), which
// the type states: only a GMPTracker has it.
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
	if recv.Success {
		return fmt.Errorf(
			"onchain: GMP Received(seq=%d) success=true for packet %s: the target call did NOT revert, so this is not an error-ack",
			packet.Sequence,
			tr.action.ID(),
		)
	}
	return nil
}

// VerifyTimedOut is the IFT timeout terminal floor: it waits (bounded) for the source chain to show the
// IFT Refunded effect (seq) — the relayer's real refund of the escrow after the deadline elapsed
// undelivered — read via the harness's own Reader, not the stub. It asserts the refund returned the exact
// amount the action escrowed, proving the user-visible outcome (the sender made whole) independent of the
// stub. The timeout/refund leg is IFT-only (a GMP action has no escrow to refund), which the type
// states: only an IFTTracker has it.
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

// StatusSource is the daemon's status endpoint viewed as a read-only cross-check surface — the one
// relayer-side surface this package consults, and only to corroborate an outcome it already observed
// on-chain. ibclink.Daemon satisfies it; declaring the one-method view here keeps this package free
// of the ibc link driver's process machinery.
type StatusSource interface {
	Status(ctx context.Context, q wire.StatusQuery) (*wire.Status, error)
}

// StatusCrossCheck queries the daemon's status API for the tracked packet and asserts it agrees with
// the on-chain reality: the packet is present, complete, carries a effect tx hash, and reports the same
// route + sequence the action claimed. It polls briefly at the destination chain's cadence, bounded by
// the destination reader's StatusRow budget (the persist lag behind an already-observed effect; see
// wire.PacketStatus), and returns an error (rather than failing a *testing.T) so the caller can
// require.NoError it.
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
			return struct{}{}, false, statusErr // transient; retry within the budget
		}
		ps, ok := status.Packet(tr.action.ID())
		if !ok {
			return struct{}{}, false, fmt.Errorf("packet %s not present in daemon status", tr.action.ID())
		}
		if ps.State != wire.PacketComplete || ps.EffectTxHash == "" {
			// Not-yet progress, retained so a timeout names the state the row was stuck in.
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

// reader resolves the per-chain Reader for chainID, or a clear error if the env bound none (e.g. a chain
// the deployment never reported fixtures for).
func (tr *packetTracker) reader(chainID string) (Reader, error) {
	return reader(tr.corr.readers, chainID)
}
