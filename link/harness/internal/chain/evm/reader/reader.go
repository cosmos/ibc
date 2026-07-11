// Package reader implements on-chain reads for EVM fixture contracts.
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
	"github.com/cosmos/ibc/link/harness/internal/onchain"

	ethereum "github.com/ethereum/go-ethereum"
)

type evmReader struct {
	client  *evm.EVMClient
	chainID string
	dep     wire.ChainDeployment
	budget  onchain.Budget
}

func New(c *evm.EVMClient, chainID string, dep wire.ChainDeployment, budget onchain.Budget) onchain.Reader {
	return &evmReader{client: c, chainID: chainID, dep: dep, budget: budget}
}

func (r *evmReader) Budget() onchain.Budget { return r.budget }

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

func (r *evmReader) GMPCount(ctx context.Context, target string) (*big.Int, error) {
	t, err := r.evmAddr("GMP target", target)
	if err != nil {
		return nil, err
	}
	return r.callUint(ctx, counterABI, t, "count")
}

func (r *evmReader) GMPDefaultPayload() []byte { return counterIncrementCalldata }

func (r *evmReader) CanonicalAddr(s string) (string, error) {
	a, err := r.evmAddr("address", s)
	if err != nil {
		return "", err
	}
	return a.Hex(), nil
}

func (r *evmReader) fixtureAddr(name string) (common.Address, error) {
	s, err := r.dep.Fixture(name)
	if err != nil {
		return common.Address{}, fmt.Errorf("onchain: chain %s: %w", r.chainID, err)
	}
	return r.evmAddr("fixture "+name, s)
}

func (r *evmReader) evmAddr(label, s string) (common.Address, error) {
	if !common.IsHexAddress(s) {
		return common.Address{}, fmt.Errorf("onchain: %s %q is not a valid EVM address", label, s)
	}
	return common.HexToAddress(s), nil
}

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
			return zero, false, err
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

func (r *evmReader) callUint(
	ctx context.Context,
	parsedABI abi.ABI,
	addr common.Address,
	method string,
	args ...any,
) (*big.Int, error) {
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
