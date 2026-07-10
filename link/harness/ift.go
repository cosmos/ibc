package harness

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/cosmos/ibc/link/harness/chain"
	"github.com/cosmos/ibc/link/harness/fixturekeys"
	"github.com/cosmos/ibc/link/harness/ibclink/wire"
	"github.com/cosmos/ibc/link/harness/internal/onchain"
	"github.com/cosmos/ibc/link/harness/topology"
)

type IFT struct {
	Route    string
	Amount   *big.Int
	Receiver string
	Timeout  time.Duration
}

type IFTPlan struct {
	run       *Session
	in        IFT
	route     wire.Route
	srcHolder string
	srcBefore *big.Int
	dstBefore *big.Int
}

type IFTOutcome struct {
	action  *onchain.IFTAction
	tracker *onchain.IFTTracker
	plan    *IFTPlan
}

func (r *Session) PrepareIFT(ctx context.Context, in IFT) (*IFTPlan, error) {
	if in.Amount == nil {
		return nil, errors.New("harness: IFT amount is required")
	}
	in.Amount = new(big.Int).Set(in.Amount)
	if in.Amount.Sign() <= 0 {
		return nil, fmt.Errorf("harness: IFT amount must be positive, got %s", in.Amount)
	}
	if in.Amount.BitLen() > 256 {
		return nil, fmt.Errorf("harness: IFT amount %s exceeds uint256", in.Amount)
	}
	route, err := requireRoute(r.h, in.Route)
	if err != nil {
		return nil, err
	}
	if in.Receiver == "" {
		recv, receiverErr := r.defaultIFTReceiver(ctx, route.Destination)
		if receiverErr != nil {
			return nil, receiverErr
		}
		in.Receiver = recv
	}
	rdr, err := r.reader(route.Destination)
	if err != nil {
		return nil, err
	}
	in.Receiver, err = rdr.CanonicalAddr(in.Receiver)
	if err != nil {
		return nil, fmt.Errorf("harness: IFT receiver: %w", err)
	}

	srcHolder, err := r.iftSourceHolder(route.Source)
	if err != nil {
		return nil, err
	}

	ift := r.ift
	srcBefore, err := ift.Balance(ctx, route.Source, srcHolder)
	if err != nil {
		return nil, err
	}
	dstBefore, err := ift.Balance(ctx, route.Destination, in.Receiver)
	if err != nil {
		return nil, err
	}

	return &IFTPlan{
		run:       r,
		in:        in,
		route:     route,
		srcHolder: srcHolder,
		srcBefore: srcBefore,
		dstBefore: dstBefore,
	}, nil
}

func (p *IFTPlan) Submit(ctx context.Context) (*IFTOutcome, error) {
	action, tracker, err := p.run.submitIFT(ctx, p.in, p.route)
	if err != nil {
		return nil, err
	}
	return &IFTOutcome{action: action, tracker: tracker, plan: p}, nil
}

// Split Prepare+Submit when baselines must precede world mutation; one-shot snapshots right before submit.
func (r *Session) IFT(ctx context.Context, in IFT) (*IFTOutcome, error) {
	plan, err := r.PrepareIFT(ctx, in)
	if err != nil {
		return nil, err
	}
	return plan.Submit(ctx)
}

func (r *Session) submitIFT(
	ctx context.Context,
	in IFT,
	route wire.Route,
) (*onchain.IFTAction, *onchain.IFTTracker, error) {
	submitter, err := r.submitter(route.Source)
	if err != nil {
		return nil, nil, err
	}
	timeoutTs, err := r.resolveTimeout(ctx, route, in.Timeout)
	if err != nil {
		return nil, nil, err
	}
	res, err := submitter.SubmitIFT(ctx, chain.IFTSubmission{
		RouteID:          route.ID,
		Receiver:         in.Receiver,
		Amount:           in.Amount,
		TimeoutTimestamp: timeoutTs,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("harness: submit IFT on route %s: %w", route.ID, err)
	}
	action := &onchain.IFTAction{
		PacketAction: onchain.PacketAction{
			RouteID:      route.ID,
			Source:       route.Source,
			Destination:  route.Destination,
			SourceTxHash: res.SourceTxHash,
			Sequence:     res.Sequence,
		},
		Receiver: in.Receiver,
		Amount:   new(big.Int).Set(in.Amount),
	}
	return action, r.packets.TrackIFT(action), nil
}

func (r *Session) submitter(chainID string) (chain.AppSubmitter, error) {
	s, ok := r.submitters[chainID]
	if !ok {
		return nil, fmt.Errorf("harness: no app submitter for chain %q", chainID)
	}
	return s, nil
}

func (r *Session) resolveTimeout(ctx context.Context, route wire.Route, timeout time.Duration) (uint64, error) {
	if timeout <= 0 {
		return 0, nil
	}
	ec, err := r.h.chains.EVM(route.Destination)
	if err != nil {
		return 0, fmt.Errorf(
			"harness: route %q IFT timeout requires EVM destination (non-EVM refund leg out of scope): %w",
			route.ID,
			err,
		)
	}
	hdr, err := ec.Client().HeaderByNumber(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("harness: read destination %s head for timeout: %w", route.Destination, err)
	}
	// Round sub-second timeouts up so they are never truncated to 0 (which would disable the leg).
	secs := uint64((timeout + time.Second - 1) / time.Second)
	return hdr.Time + secs, nil
}

func (r *Session) defaultIFTReceiver(ctx context.Context, destChainID string) (string, error) {
	rp, err := chainCapability[chain.ReceiverProvider](r.h.chains, destChainID, "ReceiverProvider")
	if err != nil {
		return "", err
	}
	return rp.NewReceiver(ctx)
}

func (r *Session) iftSourceHolder(chainID string) (string, error) {
	return r.fixture(chainID, fixturekeys.IFTFaucet)
}

func (o *IFTOutcome) iftAsserter() *onchain.IFT { return o.plan.run.ift }

func (o *IFTOutcome) status() onchain.StatusSource { return o.plan.run.h.relayer }

func (o *IFTOutcome) destProfile() topology.TimingProfile {
	return o.plan.run.h.chains.Profile(o.action.Destination)
}

func (o *IFTOutcome) srcProfile() topology.TimingProfile {
	return o.plan.run.h.chains.Profile(o.action.Source)
}

func (o *IFTOutcome) VerifyComplete(ctx context.Context) error {
	if err := o.tracker.VerifyComplete(ctx); err != nil {
		return err
	}
	if err := o.tracker.StatusCrossCheck(ctx, o.status()); err != nil {
		return err
	}
	if err := o.iftAsserter().
		VerifyReceived(ctx, o.action.Destination, o.action.Receiver, o.action.Amount, o.plan.dstBefore); err != nil {
		return err
	}
	return o.iftAsserter().VerifyEscrow(ctx, o.action.Source, o.plan.srcHolder, o.action.Amount, o.plan.srcBefore)
}

func (o *IFTOutcome) VerifyEscrowed(ctx context.Context) error {
	return o.iftAsserter().VerifyEscrow(ctx, o.action.Source, o.plan.srcHolder, o.action.Amount, o.plan.srcBefore)
}

func (o *IFTOutcome) VerifyPending(ctx context.Context) error {
	if err := o.VerifyPendingStatus(ctx); err != nil {
		return err
	}
	return o.VerifyNoMint(ctx)
}

func (o *IFTOutcome) VerifyPendingStatus(ctx context.Context) error {
	_, err := waitPacketState(ctx, o.status(), o.action.ID(), wire.PacketPending, o.destProfile())
	return err
}

func (o *IFTOutcome) VerifyPendingStable(ctx context.Context) error {
	return waitPacketStable(ctx, o.status(), o.action.ID(), wire.PacketPending, o.destProfile())
}

func (o *IFTOutcome) VerifyTimedOut(ctx context.Context) error {
	timedOut, err := waitPacketState(ctx, o.status(), o.action.ID(), wire.PacketTimedOut, o.srcProfile())
	if err != nil {
		return err
	}
	if timedOut.Reason == "" {
		return errors.New("harness: timed_out packet must carry a reason")
	}
	if timedOut.EffectTxHash == "" {
		return errors.New("harness: timed_out packet must carry an effect tx hash")
	}
	if err := o.tracker.VerifyTimedOut(ctx); err != nil {
		return err
	}
	if err := o.verifyRefunded(ctx); err != nil {
		return err
	}
	return o.VerifyNoMint(ctx)
}

func (o *IFTOutcome) VerifyNoMint(ctx context.Context) error {
	return o.iftAsserter().VerifyBalance(ctx, o.action.Destination, o.action.Receiver, o.plan.dstBefore)
}

func (o *IFTOutcome) verifyRefunded(ctx context.Context) error {
	return o.iftAsserter().VerifyBalance(ctx, o.action.Source, o.plan.srcHolder, o.plan.srcBefore)
}
