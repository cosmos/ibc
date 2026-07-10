package harness

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/cosmos/ibc/link/harness/chain"
	"github.com/cosmos/ibc/link/harness/fixturekeys"
	"github.com/cosmos/ibc/link/harness/ibclink/wire"
	"github.com/cosmos/ibc/link/harness/onchain"
	"github.com/cosmos/ibc/link/harness/topology"
)

// GMP describes one general-message call.
//
// Target is a string so the harness surface stays chain-family-agnostic: Prepare canonicalizes it
// through the destination chain's Reader, and that canonical form is what the source submission carries.
// Empty keeps the default behavior — the deployed Counter fixture (the canonical GMP target) on the
// destination chain.
type GMP struct {
	Route   string
	Target  string
	Payload []byte
}

// GMPPlan captures the frozen input and pre-submit baseline. Use when submission must happen after setup
// steps — the GMP analog of IFTPlan.
type GMPPlan struct {
	run    *Session
	in     GMP
	route  wire.Route
	before *big.Int
}

// GMPOutcome is a submitted GMP action plus verification context: the harness-native action the submitter
// produced, the typed on-chain tracker, and the plan whose frozen input/baseline the Verify methods assert
// against.
type GMPOutcome struct {
	action *onchain.GMPAction

	tracker *onchain.GMPTracker
	plan    *GMPPlan
}

// PrepareGMP resolves the route and target/payload defaults and snapshots the counter baseline before
// submit.
func (r *Session) PrepareGMP(ctx context.Context, in GMP) (*GMPPlan, error) {
	route, err := requireRoute(r.h, in.Route)
	if err != nil {
		return nil, err
	}
	rdr, err := r.reader(route.Destination)
	if err != nil {
		return nil, err
	}
	// Resolve the target. Empty defaults to the deployed Counter fixture on the destination — sourced from
	// the deployment metadata by its family-agnostic fixture name. Either way it is canonicalized through
	// the destination Reader (the family's string->address choke point), so the frozen string — what the
	// submission carries and the terminal floor exact-compares against the destination event — is the
	// family's canonical form, and a malformed target fails here rather than mid-verification.
	if in.Target == "" {
		target, fixtureErr := r.fixture(route.Destination, fixturekeys.Counter)
		if fixtureErr != nil {
			return nil, fixtureErr
		}
		in.Target = target
	}
	in.Target, err = rdr.CanonicalAddr(in.Target)
	if err != nil {
		return nil, fmt.Errorf("harness: GMP target: %w", err)
	}
	// Default payload is inherently family-specific, so it comes from the destination's Reader rather than
	// a package-level EVM call here (EVM: Counter.increment() calldata).
	if len(in.Payload) == 0 {
		in.Payload = rdr.GMPDefaultPayload()
	}
	in.Payload = append([]byte(nil), in.Payload...)
	before, err := r.gmp.Count(ctx, route.Destination, in.Target)
	if err != nil {
		return nil, err
	}
	return &GMPPlan{run: r, in: in, route: route, before: before}, nil
}

// Submit sends the call using the frozen plan input.
func (p *GMPPlan) Submit(ctx context.Context) (*GMPOutcome, error) {
	action, tracker, err := p.run.submitGMP(ctx, p.in, p.route)
	if err != nil {
		return nil, err
	}
	return &GMPOutcome{action: action, tracker: tracker, plan: p}, nil
}

// GMP is PrepareGMP + Submit in one call.
func (r *Session) GMP(ctx context.Context, in GMP) (*GMPOutcome, error) {
	plan, err := r.PrepareGMP(ctx, in)
	if err != nil {
		return nil, err
	}
	return plan.Submit(ctx)
}

// submitGMP sends the message through the source chain's submitter (see chain.AppSubmitter) and builds
// the harness-native action the tracker correlates — constructed from the harness's own submission, like
// submitIFT.
func (r *Session) submitGMP(
	ctx context.Context,
	in GMP,
	route wire.Route,
) (*onchain.GMPAction, *onchain.GMPTracker, error) {
	submitter, err := r.submitter(route.Source)
	if err != nil {
		return nil, nil, err
	}
	res, err := submitter.SubmitGMP(ctx, chain.GMPSubmission{
		RouteID: route.ID,
		Target:  in.Target,
		Payload: in.Payload,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("harness: submit GMP on route %s: %w", route.ID, err)
	}
	action := &onchain.GMPAction{
		PacketAction: onchain.PacketAction{
			RouteID:      route.ID,
			Source:       route.Source,
			Destination:  route.Destination,
			SourceTxHash: res.SourceTxHash,
			Sequence:     res.Sequence,
		},
		Target: in.Target,
	}
	return action, r.packets.TrackGMP(action), nil
}

// fixture resolves a named deployment fixture's address on chainID, wrapping the lookup to name the chain
// (the wire accessor names the fixture) so a missing fixture is a clear error, not an empty target.
func (r *Session) fixture(chainID, name string) (string, error) {
	cd, ok := r.deployment.Chain(chainID)
	if !ok {
		return "", fmt.Errorf("harness: deployment has no chain %q", chainID)
	}
	addr, err := cd.Fixture(name)
	if err != nil {
		return "", fmt.Errorf("harness: chain %s: %w", chainID, err)
	}
	return addr, nil
}

// destProfile is the timing profile of the submitted packet's destination.
func (o *GMPOutcome) destProfile() topology.TimingProfile {
	return o.plan.run.h.chains.Profile(o.action.Destination)
}

// status is the status source answering for this outcome.
func (o *GMPOutcome) status() onchain.StatusSource { return o.plan.run.h.relayer }

// VerifyComplete checks tracker terminal floor, status cross-check, and counter delta.
func (o *GMPOutcome) VerifyComplete(ctx context.Context) error {
	if err := o.tracker.VerifyComplete(ctx); err != nil {
		return err
	}
	if err := o.tracker.StatusCrossCheck(ctx, o.status()); err != nil {
		return err
	}
	return o.plan.run.gmp.VerifyTargetChangedOnce(ctx, o.action.Destination, o.action.Target, o.plan.before)
}

// VerifyPending asserts status is pending and the target counter has not changed.
func (o *GMPOutcome) VerifyPending(ctx context.Context) error {
	if err := o.VerifyPendingStatus(ctx); err != nil {
		return err
	}
	return o.VerifyTargetUnchanged(ctx)
}

// VerifyPendingStatus asserts only the daemon status.
func (o *GMPOutcome) VerifyPendingStatus(ctx context.Context) error {
	_, err := waitPacketState(ctx, o.status(), o.action.ID(), wire.PacketPending, o.destProfile())
	return err
}

// VerifyPendingStable asserts the daemon status remains pending across the destination's settle window.
func (o *GMPOutcome) VerifyPendingStable(ctx context.Context) error {
	return waitPacketStable(ctx, o.status(), o.action.ID(), wire.PacketPending, o.destProfile())
}

// VerifyErrorAck asserts the destination rejected the call and the target stayed unchanged.
func (o *GMPOutcome) VerifyErrorAck(ctx context.Context) error {
	errAck, err := waitPacketState(ctx, o.status(), o.action.ID(), wire.PacketErrorAck, o.destProfile())
	if err != nil {
		return err
	}
	if errAck.Reason == "" {
		return errors.New("harness: error_ack packet must carry a reason")
	}
	if errAck.EffectTxHash == "" {
		return errors.New("harness: error_ack packet must carry an effect tx hash")
	}
	if err := o.tracker.VerifyErrorAck(ctx); err != nil {
		return err
	}
	return o.VerifyTargetUnchanged(ctx)
}

// VerifyTargetUnchanged asserts the GMP target stayed at its prepared baseline.
func (o *GMPOutcome) VerifyTargetUnchanged(ctx context.Context) error {
	return o.plan.run.gmp.VerifyTargetUnchanged(ctx, o.action.Destination, o.action.Target, o.plan.before)
}
