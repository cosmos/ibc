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
	"github.com/cosmos/ibc/link/harness/onchain"
	"github.com/cosmos/ibc/link/harness/topology"
)

// IFT describes one fungible-token transfer. The fixture has a single token, so there is no token field.
//
// Receiver is a string so the harness surface stays chain-family-agnostic: Prepare canonicalizes it
// through the destination chain's Reader, and that canonical form is what the source action carries.
// Empty keeps the default behavior — a fresh auto-funded account on the destination chain.
type IFT struct {
	Route    string
	Amount   *big.Int
	Receiver string
	Timeout  time.Duration
}

// IFTPlan captures pre-submit baselines. Use when submission must happen after setup steps.
type IFTPlan struct {
	run       *Session
	in        IFT
	route     wire.Route
	srcHolder string
	srcBefore *big.Int
	dstBefore *big.Int
}

// IFTOutcome is a submitted IFT action plus verification context: the harness-native action the submitter
// produced, the typed on-chain tracker, and the plan whose frozen input/baselines the Verify methods
// assert against.
type IFTOutcome struct {
	action *onchain.IFTAction

	tracker *onchain.IFTTracker
	plan    *IFTPlan
}

// PrepareIFT resolves the route and snapshots balances before submit.
func (r *Session) PrepareIFT(ctx context.Context, in IFT) (*IFTPlan, error) {
	// Validate and own the amount once before dispatching to a chain family.
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
	// Resolve the receiver. Empty defaults to a fresh auto-funded account on the destination; anything
	// else is caller input. Either way it is canonicalized through the destination Reader (the family's
	// string->address choke point), so the frozen string — what the submission carries and the terminal
	// floor exact-compares against the destination event — is the family's canonical form regardless of
	// the caller's casing, and a malformed receiver fails here rather than mid-verification.
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

	// Resolve the IFT source holder — the account the source transfer debits — from that chain's deployment
	// fixture. The harness reads the same holder's balance to baseline and assert the source debit.
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

// Submit sends the transfer using the frozen plan input.
func (p *IFTPlan) Submit(ctx context.Context) (*IFTOutcome, error) {
	action, tracker, err := p.run.submitIFT(ctx, p.in, p.route)
	if err != nil {
		return nil, err
	}
	return &IFTOutcome{action: action, tracker: tracker, plan: p}, nil
}

// IFT is PrepareIFT + Submit in one call, so its balance baselines snapshot immediately before
// submission. A test that mutates the world first (pause mining, stop a node) and needs baselines from
// BEFORE the mutation must use the split form — the one-shot would silently baseline after it.
func (r *Session) IFT(ctx context.Context, in IFT) (*IFTOutcome, error) {
	plan, err := r.PrepareIFT(ctx, in)
	if err != nil {
		return nil, err
	}
	return plan.Submit(ctx)
}

// submitIFT starts the transfer through the source chain's submitter (see chain.AppSubmitter) and builds
// the harness-native action the tracker correlates — constructed from the harness's own submission, so it
// is correct by construction rather than something the SUT reports back to be verified.
func (r *Session) submitIFT(
	ctx context.Context,
	in IFT,
	route wire.Route,
) (*onchain.IFTAction, *onchain.IFTTracker, error) {
	submitter, err := r.submitter(route.Source)
	if err != nil {
		return nil, nil, err
	}
	// Resolve the timeout against the destination chain's clock (IBC v2 receive-time semantics).
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

// submitter resolves the app submitter for a source chain, or a clear error if the run bound none.
func (r *Session) submitter(chainID string) (chain.AppSubmitter, error) {
	s, ok := r.submitters[chainID]
	if !ok {
		return nil, fmt.Errorf("harness: no app submitter for chain %q", chainID)
	}
	return s, nil
}

// resolveTimeout turns a caller's relative IFT timeout into an absolute unix-second deadline read from the
// destination chain's clock, matching IBC v2 receive-time semantics. 0 (the happy path) stays 0. The
// destination chain must expose an EVM client so the harness can read its clock.
func (r *Session) resolveTimeout(ctx context.Context, route wire.Route, timeout time.Duration) (uint64, error) {
	if timeout <= 0 {
		return 0, nil
	}
	ec, err := r.h.chains.EVM(route.Destination)
	if err != nil {
		return 0, fmt.Errorf(
			"harness: route %q IFT timeout requires an EVM destination (the timeout/refund leg is out of scope for non-EVM chains): %w",
			route.ID,
			err,
		)
	}
	hdr, err := ec.Client().HeaderByNumber(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("harness: read destination %s head for timeout: %w", route.Destination, err)
	}
	// Round a sub-second duration up to the next whole second so a small timeout is never truncated to 0
	// (which would silently disable the leg the caller asked for).
	secs := uint64((timeout + time.Second - 1) / time.Second)
	return hdr.Time + secs, nil
}

// defaultIFTReceiver mints a fresh default IFT receiver on the destination chain by resolving its
// ReceiverProvider capability. A destination that advertises no ReceiverProvider fails with
// ErrCapabilityMissing.
func (r *Session) defaultIFTReceiver(ctx context.Context, destChainID string) (string, error) {
	rp, err := chainCapability[chain.ReceiverProvider](r.h.chains, destChainID, "ReceiverProvider")
	if err != nil {
		return "", err
	}
	return rp.NewReceiver(ctx)
}

// iftSourceHolder resolves the IFT source holder — the account the source transfer debits on chainID — from
// that chain's deployment fixture (fixturekeys.IFTFaucet).
func (r *Session) iftSourceHolder(chainID string) (string, error) {
	return r.fixture(chainID, fixturekeys.IFTFaucet)
}

func (o *IFTOutcome) iftAsserter() *onchain.IFT { return o.plan.run.ift }

// status is the status source answering for this outcome.
func (o *IFTOutcome) status() onchain.StatusSource { return o.plan.run.h.relayer }

// destProfile / srcProfile are the timing profiles of the submitted packet's route ends.
func (o *IFTOutcome) destProfile() topology.TimingProfile {
	return o.plan.run.h.chains.Profile(o.action.Destination)
}

func (o *IFTOutcome) srcProfile() topology.TimingProfile {
	return o.plan.run.h.chains.Profile(o.action.Source)
}

// VerifyComplete checks tracker terminal floor, status cross-check, and balance deltas.
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

// VerifyEscrowed asserts only the source-chain escrow effect.
func (o *IFTOutcome) VerifyEscrowed(ctx context.Context) error {
	return o.iftAsserter().VerifyEscrow(ctx, o.action.Source, o.plan.srcHolder, o.action.Amount, o.plan.srcBefore)
}

// VerifyPending asserts status is pending and the destination balance has not changed.
func (o *IFTOutcome) VerifyPending(ctx context.Context) error {
	if err := o.VerifyPendingStatus(ctx); err != nil {
		return err
	}
	return o.VerifyNoMint(ctx)
}

// VerifyPendingStatus asserts only the daemon status.
func (o *IFTOutcome) VerifyPendingStatus(ctx context.Context) error {
	_, err := waitPacketState(ctx, o.status(), o.action.ID(), wire.PacketPending, o.destProfile())
	return err
}

// VerifyPendingStable asserts the daemon status remains pending across the destination's settle window.
func (o *IFTOutcome) VerifyPendingStable(ctx context.Context) error {
	return waitPacketStable(ctx, o.status(), o.action.ID(), wire.PacketPending, o.destProfile())
}

// VerifyTimedOut asserts the source escrow was refunded and the receiver was never minted.
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

// VerifyNoMint asserts the destination receiver balance has not changed from the prepared baseline.
func (o *IFTOutcome) VerifyNoMint(ctx context.Context) error {
	return o.iftAsserter().VerifyBalance(ctx, o.action.Destination, o.action.Receiver, o.plan.dstBefore)
}

// verifyRefunded asserts the source faucet balance returned to its prepared baseline.
func (o *IFTOutcome) verifyRefunded(ctx context.Context) error {
	return o.iftAsserter().VerifyBalance(ctx, o.action.Source, o.plan.srcHolder, o.plan.srcBefore)
}
