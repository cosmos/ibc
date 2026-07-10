// Package reader is the EVM family's onchain.Reader: fixture-log scanning, event ABI decode, and
// bound-contract reads over one EVM chain's client. It lives beside — not inside — the evm client core so
// the provider subpackages (anvil, besu, external) that import evm for client/account primitives
// never pull the reader's fixture and ibc link wire machinery into a node launcher.
package reader

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"

	"github.com/cosmos/ibc/link/harness/chain/evm"
	"github.com/cosmos/ibc/link/harness/fixturekeys"
	"github.com/cosmos/ibc/link/harness/ibclink/wire"
	"github.com/cosmos/ibc/link/harness/onchain"

	ethereum "github.com/ethereum/go-ethereum"
)

// evmReader owns the EVM specifics of the on-chain reader: fixture-log scanning, event ABI decode,
// bound-contract reads, and hex canonicalization. It is bound to one EVM chain's concrete client and
// that chain's deployed fixture addresses; chainID labels errors (the client itself carries no logical id).
type evmReader struct {
	client  *evm.EVMClient
	chainID string
	dep     wire.ChainDeployment
	budget  onchain.Budget
}

// New builds an EVM Reader over one chain's concrete client, its deployed fixtures, and the chain's timing
// budget (completion/poll bounds for effect waits, status-row bound for the cross-check). Fixture addresses
// are resolved lazily per read (via dep.Fixture), so construction cannot fail and a missing fixture
// surfaces as a clear error naming the chain and fixture at the point of use.
func New(c *evm.EVMClient, chainID string, dep wire.ChainDeployment, budget onchain.Budget) onchain.Reader {
	return &evmReader{client: c, chainID: chainID, dep: dep, budget: budget}
}

// Budget returns the timing this reader was built with (see Reader.Budget).
func (r *evmReader) Budget() onchain.Budget { return r.budget }

// AwaitIFTReceived waits for the MockIFT destination mint carrying the route-scoped sequence for
// (routeID, seq) — the value the relayer minted under, so two routes to one receiver do not cross-match.
func (r *evmReader) AwaitIFTReceived(ctx context.Context, routeID string, seq uint64) (onchain.IFTReceived, error) {
	addr, err := r.fixtureAddr(fixturekeys.MockIFT)
	if err != nil {
		return onchain.IFTReceived{}, err
	}
	scoped := fixturekeys.RouteScopedSeq(routeID, seq)
	ev, err := waitForReceived(ctx, r.client, r.budget, addr, mockIFTABI.Events[eventIFTReceived].ID, scoped,
		decodeIFTReceived, func(e iftReceivedLog) uint64 { return e.Seq.Uint64() })
	if err != nil {
		return onchain.IFTReceived{}, err
	}
	return onchain.IFTReceived{Receiver: ev.Receiver.Hex(), Amount: ev.Amount}, nil
}

// AwaitIFTRefunded waits for the MockIFT source escrow refund carrying seq.
func (r *evmReader) AwaitIFTRefunded(ctx context.Context, seq uint64) (onchain.IFTRefunded, error) {
	addr, err := r.fixtureAddr(fixturekeys.MockIFT)
	if err != nil {
		return onchain.IFTRefunded{}, err
	}
	ev, err := waitForReceived(ctx, r.client, r.budget, addr, mockIFTABI.Events[eventIFTRefunded].ID, seq,
		decodeIFTRefunded, func(e iftRefundedLog) uint64 { return e.Seq.Uint64() })
	if err != nil {
		return onchain.IFTRefunded{}, err
	}
	return onchain.IFTRefunded{Amount: ev.Amount}, nil
}

// AwaitGMPReceived waits for the MockGMP destination delivery carrying the route-scoped sequence for
// (routeID, seq) — the value the relayer delivered under, so two routes to one target do not cross-match.
func (r *evmReader) AwaitGMPReceived(ctx context.Context, routeID string, seq uint64) (onchain.GMPReceived, error) {
	addr, err := r.fixtureAddr(fixturekeys.MockGMP)
	if err != nil {
		return onchain.GMPReceived{}, err
	}
	scoped := fixturekeys.RouteScopedSeq(routeID, seq)
	ev, err := waitForReceived(ctx, r.client, r.budget, addr, mockGMPABI.Events[eventGMPReceived].ID, scoped,
		decodeGMPReceived, func(e gmpReceivedLog) uint64 { return e.Seq.Uint64() })
	if err != nil {
		return onchain.GMPReceived{}, err
	}
	return onchain.GMPReceived{Target: ev.Target.Hex(), Success: ev.Success}, nil
}

// IFTBalance reads MockIFT.balanceOf(holder) at the chain's deployed MockIFT fixture.
func (r *evmReader) IFTBalance(ctx context.Context, holder string) (*big.Int, error) {
	addr, err := r.fixtureAddr(fixturekeys.MockIFT)
	if err != nil {
		return nil, err
	}
	h, err := r.evmAddr("IFT holder", holder)
	if err != nil {
		return nil, err
	}
	return r.callUint(ctx, mockIFTABI, addr, "balanceOf", h)
}

// GMPCount reads Counter.count() at target (the GMP delivery target — the deployed Counter by default).
func (r *evmReader) GMPCount(ctx context.Context, target string) (*big.Int, error) {
	t, err := r.evmAddr("GMP target", target)
	if err != nil {
		return nil, err
	}
	return r.callUint(ctx, counterABI, t, "count")
}

// GMPDefaultPayload returns the EVM default GMP payload: Counter.increment() calldata.
func (r *evmReader) GMPDefaultPayload() []byte { return counterIncrementCalldata }

// CanonicalAddr validates s as an EVM hex address and returns its EIP-55 checksummed form — the family's
// canonical string (see Reader.CanonicalAddr), so casing variants of one address compare equal.
func (r *evmReader) CanonicalAddr(s string) (string, error) {
	a, err := r.evmAddr("address", s)
	if err != nil {
		return "", err
	}
	return a.Hex(), nil
}

// fixtureAddr resolves a named fixture's EVM address on this reader's chain, wrapping the deployment
// lookup error to name the chain (the wire accessor names the fixture) so a missing fixture is a clear
// "chain X has no fixture Y" error, never a zero address that silently "reads".
func (r *evmReader) fixtureAddr(name string) (common.Address, error) {
	s, err := r.dep.Fixture(name)
	if err != nil {
		return common.Address{}, fmt.Errorf("onchain: chain %s: %w", r.chainID, err)
	}
	return r.evmAddr("fixture "+name, s)
}

// evmAddr parses a family-native (hex) address string, rejecting a malformed one rather than letting
// common.HexToAddress silently coerce it (which pads/truncates without error). This is the EVM reader's
// single string->address choke point — where a family-native address handed to the Reader is bound to the
// EVM family; label names the offending value in the error.
func (r *evmReader) evmAddr(label, s string) (common.Address, error) {
	if !common.IsHexAddress(s) {
		return common.Address{}, fmt.Errorf("onchain: %s %q is not a valid EVM address", label, s)
	}
	return common.HexToAddress(s), nil
}

// waitForReceived polls c's logs for `addr` matching `topic` until a decoded event whose sequence equals
// wantSeq appears, returning it. Bounded by ctx and the chain's completion budget, polling at the chain's
// cadence (budget). Shared by the IFT and GMP effect waits (FromBlock 0 full scan: the effect may already
// have landed under no --wait, and the log volume is tiny, so a per-poll scan avoids cursor bookkeeping).
func waitForReceived[T any](
	ctx context.Context,
	c *evm.EVMClient,
	budget onchain.Budget,
	addr common.Address,
	topic common.Hash,
	wantSeq uint64,
	decode func([]byte) (T, error),
	seqOf func(T) uint64,
) (T, error) {
	q := ethereum.FilterQuery{
		FromBlock: big.NewInt(0),
		Addresses: []common.Address{addr},
		Topics:    [][]common.Hash{{topic}},
	}

	desc := fmt.Sprintf("event(seq=%d) on %s", wantSeq, addr.Hex())
	return onchain.Await(ctx, budget.Completion, budget.Poll, desc, func(ctx context.Context) (T, bool, error) {
		var zero T
		logs, err := c.Logs(ctx, q)
		if err != nil {
			return zero, false, err // transient RPC hiccup; retry within the budget
		}
		for _, lg := range logs {
			ev, derr := decode(lg.Data)
			if derr != nil {
				return zero, true, derr
			}
			if seqOf(ev) == wantSeq {
				return ev, true, nil
			}
		}
		return zero, false, nil
	})
}

// callUint invokes a read-only method returning a single uint (e.g. balanceOf, count) on the contract at
// addr (parsedABI) via the reader's client and returns the *big.Int result. It centralizes the
// bind.NewBoundContract + Call + out[0].(*big.Int) dance the IFT and GMP reads both perform.
func (r *evmReader) callUint(
	ctx context.Context,
	parsedABI abi.ABI,
	addr common.Address,
	method string,
	args ...any,
) (*big.Int, error) {
	// caller-only bound contract (nil transactor/filterer): *ethclient.Client satisfies the caller.
	bound := bind.NewBoundContract(addr, parsedABI, r.client.Client(), nil, nil)
	var out []any
	if err := bound.Call(&bind.CallOpts{Context: ctx}, &out, method, args...); err != nil {
		return nil, fmt.Errorf("onchain: call %s on %s: %w", method, r.chainID, err)
	}
	n, ok := out[0].(*big.Int)
	if !ok {
		return nil, fmt.Errorf("onchain: %s returned %T, want *big.Int", method, out[0])
	}
	return n, nil
}
