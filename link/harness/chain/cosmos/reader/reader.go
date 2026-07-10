// Package reader is the cosmos family's onchain.Reader: CometBFT tx_search correlation and typed gRPC
// bank reads over one cosmos chain's client. It lives beside — not inside — the cosmos chain core so the
// launch path (provision -> cosmos -> sandboxd) never pulls the reader's ibc link wire machinery into a
// node launcher, mirroring the evm/reader split.
package reader

import (
	"context"
	"fmt"
	"math/big"

	"github.com/cosmos/ibc/link/harness/chain/cosmos"
	"github.com/cosmos/ibc/link/harness/fixturekeys"
	"github.com/cosmos/ibc/link/harness/ibclink/wire"
	"github.com/cosmos/ibc/link/harness/onchain"
)

// cosmosReader is the cosmos-family implementation of Reader: it observes a real Cosmos SDK chain over
// CometBFT tx_search for delivery correlation and typed gRPC bank balance for token state. It is bound to one
// cosmos chain and its deployment. The correlator, app asserters, and harness surface stay
// family-agnostic.
//
// IFT and GMP deliveries are native 27-gmp IBC v2 receives, correlated by write_acknowledgement. IFT
// received/refunded effects come from the native IFT module's events; GMP success still reads the inner bank
// transfer and counter-denom balance.
type cosmosReader struct {
	client  *cosmos.Client
	dep     wire.ChainDeployment
	chainID string
	budget  onchain.Budget
}

// New builds a cosmos Reader over one chain's concrete read client (resolved via the cosmos.ClientProvider
// capability, so any cosmos-family chain can bind one), its deployment, and the chain's timing budget. It
// borrows the chain-owned client (closed by the chain's Stop, never here); chainID labels errors. The
// deployment is retained so a missing client/denom fixture surfaces as a clear error at the point of use,
// mirroring the EVM reader.
func New(c *cosmos.Client, chainID string, dep wire.ChainDeployment, budget onchain.Budget) onchain.Reader {
	return &cosmosReader{client: c, dep: dep, chainID: chainID, budget: budget}
}

// Budget returns the timing this reader was built with (see Reader.Budget).
func (r *cosmosReader) Budget() onchain.Budget { return r.budget }

// AwaitIFTReceived waits for the native IFT mint paired with the route-scoped destination sequence.
func (r *cosmosReader) AwaitIFTReceived(ctx context.Context, routeID string, seq uint64) (onchain.IFTReceived, error) {
	denom, err := r.fixture(fixturekeys.IFTDenom)
	if err != nil {
		return onchain.IFTReceived{}, err
	}
	destClient, err := r.fixture(fixturekeys.AttestationsClient)
	if err != nil {
		return onchain.IFTReceived{}, err
	}
	scoped := fixturekeys.RouteScopedSeq(routeID, seq)
	desc := fmt.Sprintf("cosmos %s Received(seq=%d) on %s", wire.AppTypeIFT, seq, r.chainID)
	return onchain.Await(
		ctx,
		r.budget.Completion,
		r.budget.Poll,
		desc,
		func(ctx context.Context) (onchain.IFTReceived, bool, error) {
			recvs, err := r.client.IFTRecvs(ctx, destClient)
			if err != nil {
				return onchain.IFTReceived{}, false, err // transient RPC hiccup; retry within the budget
			}
			for _, recv := range recvs {
				if recv.Seq != scoped {
					continue
				}
				if recv.Denom != denom {
					return onchain.IFTReceived{}, true, fmt.Errorf(
						"onchain: cosmos IFT Received(seq=%d) on %s carried denom %q, want %q",
						seq,
						r.chainID,
						recv.Denom,
						denom,
					)
				}
				return onchain.IFTReceived{Receiver: recv.Receiver, Amount: recv.Amount}, true, nil
			}
			return onchain.IFTReceived{}, false, nil
		},
	)
}

// AwaitIFTRefunded waits for the native IFT callback's refund event.
func (r *cosmosReader) AwaitIFTRefunded(ctx context.Context, seq uint64) (onchain.IFTRefunded, error) {
	denom, err := r.fixture(fixturekeys.IFTDenom)
	if err != nil {
		return onchain.IFTRefunded{}, err
	}
	sourceClient, err := r.fixture(fixturekeys.AttestationsClient)
	if err != nil {
		return onchain.IFTRefunded{}, err
	}
	desc := fmt.Sprintf("cosmos %s Refunded(seq=%d) on %s", wire.AppTypeIFT, seq, r.chainID)
	return onchain.Await(
		ctx,
		r.budget.Completion,
		r.budget.Poll,
		desc,
		func(ctx context.Context) (onchain.IFTRefunded, bool, error) {
			refunds, queryErr := r.client.IFTRefunds(ctx, sourceClient)
			if queryErr != nil {
				return onchain.IFTRefunded{}, false, queryErr
			}
			for _, refund := range refunds {
				if refund.Seq != seq {
					continue
				}
				if refund.Denom != denom {
					return onchain.IFTRefunded{}, true, fmt.Errorf(
						"onchain: cosmos IFT Refunded(seq=%d) on %s carried denom %q, want %q",
						seq, r.chainID, refund.Denom, denom,
					)
				}
				return onchain.IFTRefunded{Amount: refund.Amount}, true, nil
			}
			return onchain.IFTRefunded{}, false, nil
		},
	)
}

// AwaitGMPReceived waits (bounded) for the destination chain's real 27-gmp recv for seq and returns its
// normalized view. The IBC v2 recv into the chain's 27-gmp module is correlated by the module's own
// write_acknowledgement event (dest client + sequence), with success decoded from the canonical ack bytes
// and the target read from the module's inner bank transfer on success (an error ack credits nothing, so it
// carries no target). Reading straight from the module's events keeps the reader independent of the stub.
func (r *cosmosReader) AwaitGMPReceived(ctx context.Context, routeID string, seq uint64) (onchain.GMPReceived, error) {
	destClient, err := r.fixture(fixturekeys.AttestationsClient)
	if err != nil {
		return onchain.GMPReceived{}, err
	}
	ics27, err := r.fixture(fixturekeys.ICS27Account)
	if err != nil {
		return onchain.GMPReceived{}, err
	}
	gmpDenom, err := r.fixture(fixturekeys.GMPDenom)
	if err != nil {
		return onchain.GMPReceived{}, err
	}

	// The relayer fabricates the IBC v2 packet at the route-scoped sequence, so match on that value.
	scoped := fixturekeys.RouteScopedSeq(routeID, seq)
	desc := fmt.Sprintf("cosmos GMP Received(seq=%d) on %s", seq, r.chainID)
	return onchain.Await(
		ctx,
		r.budget.Completion,
		r.budget.Poll,
		desc,
		func(ctx context.Context) (onchain.GMPReceived, bool, error) {
			recvs, err := r.client.GMPRecvs(ctx, destClient, ics27, gmpDenom)
			if err != nil {
				return onchain.GMPReceived{}, false, err // transient RPC hiccup; retry within the budget
			}
			for _, rec := range recvs {
				if rec.Seq != scoped {
					continue
				}
				// On a success ack the target is read from the module's inner bank transfer. An error ack moved
				// nothing, so there is no inner transfer to read the target from and rec.Target is empty — the
				// error-ack floor (VerifyErrorAck) does not assert the target for exactly this reason.
				return onchain.GMPReceived{Target: rec.Target, Success: rec.Success}, true, nil
			}
			return onchain.GMPReceived{}, false, nil
		},
	)
}

// IFTBalance reads holder's bank balance of the deployment's IFT denom on the cosmos chain. The receiver
// string is verbatim, family-native (a bech32 account address).
func (r *cosmosReader) IFTBalance(ctx context.Context, holder string) (*big.Int, error) {
	denom, err := r.fixture(fixturekeys.IFTDenom)
	if err != nil {
		return nil, err
	}
	return r.client.BalanceOf(ctx, holder, denom)
}

// GMPCount reads the cosmos GMP "counter" — the target's bank balance of the deployment's GMP denom
// (fixturekeys.GMPDenom). One increment is exactly one <gmpDenom> escrow->target send, so this balance is the
// count (counter-as-balance). A never-incremented target holds nothing, which BalanceOf reports as zero.
func (r *cosmosReader) GMPCount(ctx context.Context, target string) (*big.Int, error) {
	denom, err := r.fixture(fixturekeys.GMPDenom)
	if err != nil {
		return nil, err
	}
	return r.client.BalanceOf(ctx, target, denom)
}

// GMPDefaultPayload returns the cosmos default GMP payload — the fixed "increment" convention the executor
// recognizes (the cosmos analog of the EVM Counter.increment() calldata). It is inherently family-specific,
// so it lives behind the reader rather than as a package-level EVM call in the harness's gmp.go.
func (r *cosmosReader) GMPDefaultPayload() []byte { return []byte(cosmos.GMPIncrementPayload) }

// CanonicalAddr validates s as a cosmos bech32 account address and returns its canonical (re-encoded)
// form — the family's canonical string (see Reader.CanonicalAddr), delegated to the family's own
// address choke point.
func (r *cosmosReader) CanonicalAddr(s string) (string, error) {
	addr, err := cosmos.CanonicalAddress(s)
	if err != nil {
		return "", fmt.Errorf("onchain: %w", err)
	}
	return addr, nil
}

// fixture resolves a named fixture on this reader's cosmos deployment, wrapping the lookup error to name
// the chain (the wire accessor names the fixture).
func (r *cosmosReader) fixture(name string) (string, error) {
	v, err := r.dep.Fixture(name)
	if err != nil {
		return "", fmt.Errorf("onchain: chain %s: %w", r.chainID, err)
	}
	return v, nil
}
