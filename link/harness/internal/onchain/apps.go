package onchain

import (
	"context"
	"fmt"
	"math/big"
)

func NewIFT(readers map[string]Reader) *IFT { return &IFT{readers: readers} }

func NewGMP(readers map[string]Reader) *GMP { return &GMP{readers: readers} }

func reader(readers map[string]Reader, chainID string) (Reader, error) {
	rdr, ok := readers[chainID]
	if !ok {
		return nil, fmt.Errorf("onchain: no reader for chain %q", chainID)
	}
	return rdr, nil
}

type IFT struct {
	readers map[string]Reader
}

func (i *IFT) Balance(ctx context.Context, chainID, holder string) (*big.Int, error) {
	rdr, err := reader(i.readers, chainID)
	if err != nil {
		return nil, err
	}
	return rdr.IFTBalance(ctx, holder)
}

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

type GMP struct {
	readers map[string]Reader
}

func (g *GMP) Count(ctx context.Context, chainID, target string) (*big.Int, error) {
	rdr, err := reader(g.readers, chainID)
	if err != nil {
		return nil, err
	}
	return rdr.GMPCount(ctx, target)
}

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
