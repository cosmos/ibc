package onchain

import (
	"context"
	"fmt"
	"math/big"
)

// The IFT and GMP asserters do the app-level delta/exact-value math (escrow decreased by amount, target
// advanced by exactly one, ...) over values each Reader reads from its own chain, so the assertion
// semantics live here while the chain I/O stays behind the family-agnostic Reader seam.

// NewIFT builds the IFT asserter over the env's per-chain Readers (keyed by Chain.ID).
func NewIFT(readers map[string]Reader) *IFT { return &IFT{readers: readers} }

// NewGMP builds the GMP asserter over the env's per-chain Readers (keyed by Chain.ID).
func NewGMP(readers map[string]Reader) *GMP { return &GMP{readers: readers} }

// reader resolves the per-chain Reader for chainID, or a clear error if none is bound.
func reader(readers map[string]Reader, chainID string) (Reader, error) {
	rdr, ok := readers[chainID]
	if !ok {
		return nil, fmt.Errorf("onchain: no reader for chain %q", chainID)
	}
	return rdr, nil
}

// IFT reads IFT token state via each chain's Reader (holder as a family-native address string).
type IFT struct {
	readers map[string]Reader
}

// Balance reads the IFT balance of holder on chain chainID. Exposed so a test can snapshot a pre-action
// balance (e.g. the source sender's, for VerifyEscrow).
func (i *IFT) Balance(ctx context.Context, chainID, holder string) (*big.Int, error) {
	rdr, err := reader(i.readers, chainID)
	if err != nil {
		return nil, err
	}
	return rdr.IFTBalance(ctx, holder)
}

// VerifyBalance asserts the IFT balance of holder on chainID equals want. The terminal IFT check: after a
// completed transfer, the destination receiver holds exactly the transferred amount.
func (i *IFT) VerifyBalance(ctx context.Context, chainID, holder string, want *big.Int) error {
	got, err := i.Balance(ctx, chainID, holder)
	if err != nil {
		return err
	}
	if got.Cmp(want) != 0 {
		return fmt.Errorf("onchain: IFT balance of %s on %s: got %s, want %s", holder, chainID, got, want)
	}
	return nil
}

// VerifyEscrow asserts the source sender's IFT balance decreased by exactly amount relative to the before
// snapshot — the escrow the fixture's sendTransfer performs on the source side. The caller captures before
// via Balance prior to the transfer.
func (i *IFT) VerifyEscrow(ctx context.Context, chainID, sender string, amount, before *big.Int) error {
	want := new(big.Int).Sub(before, amount)
	got, err := i.Balance(ctx, chainID, sender)
	if err != nil {
		return err
	}
	if got.Cmp(want) != 0 {
		return fmt.Errorf("onchain: IFT escrow on %s: balance of %s got %s, want before(%s)-amount(%s)=%s",
			chainID, sender, got, before, amount, want)
	}
	return nil
}

// VerifyReceived asserts the destination receiver's IFT balance increased by exactly amount relative to
// the before snapshot — the mint the fixture's receiveTransfer performs on the destination side.
func (i *IFT) VerifyReceived(ctx context.Context, chainID, receiver string, amount, before *big.Int) error {
	want := new(big.Int).Add(before, amount)
	got, err := i.Balance(ctx, chainID, receiver)
	if err != nil {
		return err
	}
	if got.Cmp(want) != 0 {
		return fmt.Errorf("onchain: IFT received on %s: balance of %s got %s, want before(%s)+amount(%s)=%s",
			chainID, receiver, got, before, amount, want)
	}
	return nil
}

// GMP reads delivery-target state via each chain's Reader, so the exactly-once outcome is corroborated
// independently of the black box. It is the GMP analog of IFT: where IFT asserts a token balance, GMP
// asserts the GMP target's observable state changed by exactly the delta one delivery produces (see
// VerifyTargetChangedOnce).
type GMP struct {
	readers map[string]Reader
}

// Count reads the GMP target's Counter state at target on chain chainID. Exposed so a test snapshots the
// pre-action count (the baseline VerifyTargetChangedOnce measures the delta against).
func (g *GMP) Count(ctx context.Context, chainID, target string) (*big.Int, error) {
	rdr, err := reader(g.readers, chainID)
	if err != nil {
		return nil, err
	}
	return rdr.GMPCount(ctx, target)
}

// VerifyTargetChangedOnce asserts the GMP target's state at target on chainID advanced by exactly +1
// relative to before — the exactly-once wire. "Once" is the count of deliveries: the relayer must execute
// the target effect precisely once. A mismatch means the relayer either double-delivered (+2) or missed the
// delivery entirely (+0). The caller captures before via Count prior to the GMP action.
func (g *GMP) VerifyTargetChangedOnce(ctx context.Context, chainID, target string, before *big.Int) error {
	got, err := g.Count(ctx, chainID, target)
	if err != nil {
		return err
	}
	want := new(big.Int).Add(before, big.NewInt(1))
	if got.Cmp(want) != 0 {
		return fmt.Errorf(
			"onchain: Counter %s on %s: got %s, want before(%s)+1=%s",
			target,
			chainID,
			got,
			before,
			want,
		)
	}
	return nil
}

// VerifyTargetUnchanged asserts the GMP target's state at target on chainID is UNCHANGED from before — the
// error-ack wire. When a GMP message is delivered but its target execution fails (e.g. invalid payload), the
// relayer marks the packet error_ack and the target state must not have moved: the delivery happened (GMP
// Received fired with success=false) but had no target effect. The caller captures before via Count prior
// to the GMP action.
func (g *GMP) VerifyTargetUnchanged(ctx context.Context, chainID, target string, before *big.Int) error {
	got, err := g.Count(ctx, chainID, target)
	if err != nil {
		return err
	}
	if got.Cmp(before) != 0 {
		return fmt.Errorf(
			"onchain: Counter %s on %s: got %s, want unchanged %s — an error-ack delivery must not change target state",
			target,
			chainID,
			got,
			before,
		)
	}
	return nil
}
