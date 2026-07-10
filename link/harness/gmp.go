package harness

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/cosmos/ibc/link/harness/chain"
	"github.com/cosmos/ibc/link/harness/fixturekeys"
	"github.com/cosmos/ibc/link/harness/ibclink/wire"
	"github.com/cosmos/ibc/link/harness/internal/onchain"
	"github.com/cosmos/ibc/link/harness/topology"
)

type GMP struct {
	Route   string
	Target  string
	Payload []byte
}

type GMPPlan struct {
	run    *Session
	in     GMP
	route  wire.Route
	before *big.Int
}

type GMPOutcome struct {
	action  *onchain.GMPAction
	tracker *onchain.GMPTracker
	plan    *GMPPlan
}

func (r *Session) PrepareGMP(ctx context.Context, in GMP) (*GMPPlan, error) {
	route, err := requireRoute(r.h, in.Route)
	if err != nil {
		return nil, err
	}
	rdr, err := r.reader(route.Destination)
	if err != nil {
		return nil, err
	}
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

func (p *GMPPlan) Submit(ctx context.Context) (*GMPOutcome, error) {
	action, tracker, err := p.run.submitGMP(ctx, p.in, p.route)
	if err != nil {
		return nil, err
	}
	return &GMPOutcome{action: action, tracker: tracker, plan: p}, nil
}

func (r *Session) GMP(ctx context.Context, in GMP) (*GMPOutcome, error) {
	plan, err := r.PrepareGMP(ctx, in)
	if err != nil {
		return nil, err
	}
	return plan.Submit(ctx)
}

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

func (o *GMPOutcome) destProfile() topology.TimingProfile {
	return o.plan.run.h.chains.Profile(o.action.Destination)
}

func (o *GMPOutcome) status() onchain.StatusSource { return o.plan.run.h.relayer }

func (o *GMPOutcome) VerifyComplete(ctx context.Context) error {
	if err := o.tracker.VerifyComplete(ctx); err != nil {
		return err
	}
	if err := o.tracker.StatusCrossCheck(ctx, o.status()); err != nil {
		return err
	}
	return o.plan.run.gmp.VerifyTargetChangedOnce(ctx, o.action.Destination, o.action.Target, o.plan.before)
}

func (o *GMPOutcome) VerifyPending(ctx context.Context) error {
	if err := o.VerifyPendingStatus(ctx); err != nil {
		return err
	}
	return o.VerifyTargetUnchanged(ctx)
}

func (o *GMPOutcome) VerifyPendingStatus(ctx context.Context) error {
	_, err := waitPacketState(ctx, o.status(), o.action.ID(), wire.PacketPending, o.destProfile())
	return err
}

func (o *GMPOutcome) VerifyPendingStable(ctx context.Context) error {
	return waitPacketStable(ctx, o.status(), o.action.ID(), wire.PacketPending, o.destProfile())
}

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

func (o *GMPOutcome) VerifyTargetUnchanged(ctx context.Context) error {
	return o.plan.run.gmp.VerifyTargetUnchanged(ctx, o.action.Destination, o.action.Target, o.plan.before)
}
