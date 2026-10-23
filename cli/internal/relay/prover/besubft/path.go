// SPDX-License-Identifier: Apache-2.0

package besubft

import (
	"context"
	"fmt"
	"slices"

	"github.com/cosmos/ibc-contracts/packages/go-abigen/besumsgs"
	"github.com/cosmos/ibc/cli/besu"
)

// stuckError is where the search from the client's trusted height stops: the
// header after reached does not verify from it.
type stuckError struct {
	trusted  uint64
	reached  *besu.ParsedHeader
	signed   int
	required int
}

func (e *stuckError) Error() string {
	return fmt.Sprintf(
		"client at height %d cannot follow the validator set past height %d: "+
			"header %d is sealed by %d of the %d validators trusted at height %d, it needs %d",
		e.trusted, e.reached.Height,
		e.reached.Height+1, e.signed, len(e.reached.Validators), e.reached.Height, e.required,
	)
}

// path returns the headers that take the client from trusted to target, each
// verifiable from the one before, each the furthest the bisection finds. Header
// voting changes at most one validator a block, which leaves every header
// verifiable from its parent at a trust level of at most one half, so the
// search reaches target. Otherwise it may stop short, at a header that is not.
func (g *Generator) path(
	ctx context.Context,
	trusted, target *besu.ParsedHeader,
	level besumsgs.IBesuLightClientMsgsTrustThreshold,
) ([]*besu.ParsedHeader, *stuckError, error) {
	var steps []*besu.ParsedHeader
	from := trusted
	for !verifies(from, target, level) {
		lo, hi := from, target
		for hi.Height-lo.Height > 1 {
			mid, err := g.sealedHeader(ctx, lo.Height+(hi.Height-lo.Height)/2)
			if err != nil {
				return nil, nil, err
			}
			if verifies(from, mid, level) {
				lo = mid
			} else {
				hi = mid
			}
		}
		if lo == from {
			return steps, &stuckError{
				trusted:  trusted.Height,
				reached:  from,
				signed:   trustedSigners(from, hi),
				required: required(from, level),
			}, nil
		}
		steps = append(steps, lo)
		from = lo
	}
	return append(steps, target), nil, nil
}

func (g *Generator) sealedHeader(ctx context.Context, height uint64) (*besu.ParsedHeader, error) {
	header, err := g.counterparty.SealedHeader(ctx, height)
	if err != nil {
		return nil, fmt.Errorf("reading header %d: %w", height, err)
	}
	return header, nil
}

// verifies reports whether the light client accepts header trusting from.
func verifies(from, header *besu.ParsedHeader, level besumsgs.IBesuLightClientMsgsTrustThreshold) bool {
	return trustedSigners(from, header) >= required(from, level)
}

// required is how many of from's validators must seal an update trusting
// from, ceil(n * level) as the contract computes it.
func required(from *besu.ParsedHeader, level besumsgs.IBesuLightClientMsgsTrustThreshold) int {
	n := len(from.Validators) * int(level.Numerator)
	d := int(level.Denominator)
	return (n + d - 1) / d
}

// trustedSigners is how many of from's validators sealed header.
func trustedSigners(from, header *besu.ParsedHeader) int {
	n := 0
	for _, signer := range header.Signers {
		if slices.Contains(from.Validators, signer) {
			n++
		}
	}
	return n
}
