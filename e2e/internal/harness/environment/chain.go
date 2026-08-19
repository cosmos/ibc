// SPDX-License-Identifier: Apache-2.0

package environment

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	chainimpl "github.com/cosmos/ibc/e2e/internal/harness/chain"
	chainevm "github.com/cosmos/ibc/e2e/internal/harness/chain/evm"
)

var (
	ErrCapabilityUnavailable = errors.New("environment: capability unavailable")
	ErrEnvironmentClosed     = errors.New("environment: resolved handles cannot be used after Environment.Close")
)

const capabilityCleanupTimeout = 15 * time.Second

// Chain is the resolved workflow-facing view of a Chain declaration. Adapter
// values and control interfaces stay private so attachment cannot accidentally
// inherit controls from the implementation used to connect to it.
type Chain struct {
	id         ChainID
	evmChainID uint64
	rpcURL     string
	wsURL      string
	timing     Timing
	impl       chainimpl.Chain
	lease      *environmentLease
	mining     *Mining
	node       *NodeLifecycle
	funding    *Funding
}

func (c *Chain) bindLease(lease *environmentLease) {
	c.lease = lease
	if c.mining != nil {
		c.mining.chain = c
	}
	if c.node != nil {
		c.node.chain = c
	}
	if c.funding != nil {
		c.funding.chain = c
	}
}

func (c *Chain) use(use func() error) error {
	err := c.lease.use(use)
	if errors.Is(err, ErrEnvironmentClosed) {
		err = fmt.Errorf("%w: Chain %q", err, c.id)
	}
	return err
}

func (c *Chain) ID() ChainID        { return c.id }
func (c *Chain) EVMChainID() uint64 { return c.evmChainID }
func (c *Chain) RPCURL() string     { return c.rpcURL }
func (c *Chain) Timing() Timing     { return c.timing }

// WSURL is empty for Chains whose adapter serves no websocket endpoint;
// auto-relayed routes cannot source from those.
func (c *Chain) WSURL() string { return c.wsURL }

func (c *Chain) Height(ctx context.Context) (uint64, error) {
	var height uint64
	err := c.use(func() error {
		var err error
		height, err = c.impl.Height(ctx)
		return err
	})
	return height, err
}

// EVM returns non-owning EVM operations. The Environment keeps exclusive
// control of the underlying client lifecycle.
func (c *Chain) EVM() (*EVM, error) {
	err := c.use(func() error {
		ok, err := chainevm.WithChainClient(c.impl, func(*chainevm.EVMClient) error { return nil })
		if !ok {
			return fmt.Errorf("%w: Chain %q has no EVM access", ErrCapabilityUnavailable, c.id)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return &EVM{chain: c}, nil
}

// EVM exposes operations on an EVM Chain without exposing the owned client's
// Close method or its raw RPC client.
type EVM struct {
	chain *Chain
}

func (e *EVM) WaitNextPendingTx(ctx context.Context) error {
	return e.use(func(client *chainevm.EVMClient) error {
		return client.WaitNextPendingTx(ctx, e.transactionWait())
	})
}

func (e *EVM) BroadcastTx(
	ctx context.Context,
	from chainevm.Account,
	to *common.Address,
	data []byte,
	value *big.Int,
) (*types.Receipt, error) {
	var receipt *types.Receipt
	err := e.use(func(client *chainevm.EVMClient) error {
		var err error
		receipt, err = client.BroadcastTx(ctx, e.transactionWait(), from, to, data, value)
		return err
	})
	return receipt, err
}

func (e *EVM) TransactionReceipt(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
	var receipt *types.Receipt
	err := e.use(func(client *chainevm.EVMClient) error {
		var err error
		receipt, err = client.Client().TransactionReceipt(ctx, hash)
		return err
	})
	return receipt, err
}

func (e *EVM) TransactionByHash(ctx context.Context, hash common.Hash) (*types.Transaction, bool, error) {
	var tx *types.Transaction
	var pending bool
	err := e.use(func(client *chainevm.EVMClient) error {
		var err error
		tx, pending, err = client.Client().TransactionByHash(ctx, hash)
		return err
	})
	return tx, pending, err
}

// AwaitTransactionReceipt polls until hash is mined using the Chain's timing policy.
func (e *EVM) AwaitTransactionReceipt(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
	var receipt *types.Receipt
	err := e.use(func(client *chainevm.EVMClient) error {
		var err error
		receipt, err = client.WaitForReceipt(ctx, hash, e.transactionWait())
		return err
	})
	return receipt, err
}

func (e *EVM) transactionWait() chainevm.TransactionWait {
	return chainevm.TransactionWait{
		Timeout:      e.chain.timing.CompletionBudget,
		PollInterval: e.chain.timing.PollInterval,
	}
}

func (e *EVM) Logs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error) {
	var logs []types.Log
	err := e.use(func(client *chainevm.EVMClient) error {
		var err error
		logs, err = client.Logs(ctx, query)
		return err
	})
	return logs, err
}

func (e *EVM) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	var header *types.Header
	err := e.use(func(client *chainevm.EVMClient) error {
		var err error
		header, err = client.Client().HeaderByNumber(ctx, number)
		return err
	})
	return header, err
}

func (e *EVM) CodeAt(ctx context.Context, address common.Address, number *big.Int) ([]byte, error) {
	var code []byte
	err := e.use(func(client *chainevm.EVMClient) error {
		var err error
		code, err = client.Client().CodeAt(ctx, address, number)
		return err
	})
	return code, err
}

func (e *EVM) BalanceAt(ctx context.Context, address common.Address, number *big.Int) (*big.Int, error) {
	var balance *big.Int
	err := e.use(func(client *chainevm.EVMClient) error {
		var err error
		balance, err = client.Client().BalanceAt(ctx, address, number)
		return err
	})
	return balance, err
}

// UseContractCaller lends the active EVM client as a read-only generated
// contract backend. The client remains owned by the environment.
func (e *EVM) UseContractCaller(use func(bind.ContractCaller) error) error {
	return e.use(func(client *chainevm.EVMClient) error {
		return use(client.Client())
	})
}

func (e *EVM) use(use func(*chainevm.EVMClient) error) error {
	return e.chain.use(func() error {
		ok, err := chainevm.WithChainClient(e.chain.impl, use)
		if !ok {
			return fmt.Errorf("%w: Chain %q has no EVM access", ErrCapabilityUnavailable, e.chain.id)
		}
		return err
	})
}

// Mining returns explicit mining control only when the selected declaration
// and adapter guarantee it. Attached Chains never receive it from connectivity alone.
func (c *Chain) Mining() (*Mining, error) {
	err := c.use(func() error {
		if c.mining == nil {
			return fmt.Errorf("%w: Chain %q has no mining control", ErrCapabilityUnavailable, c.id)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return c.mining, nil
}

// NodeLifecycle returns explicit node fault control only for environment-owned
// adapters that can safely stop and restart their node.
func (c *Chain) NodeLifecycle() (*NodeLifecycle, error) {
	err := c.use(func() error {
		if c.node == nil {
			return fmt.Errorf("%w: Chain %q has no node lifecycle control", ErrCapabilityUnavailable, c.id)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return c.node, nil
}

// Funding returns native-token funding only for managed Chains whose adapter
// can guarantee and verify a minimum externally owned account balance.
// Attached Chains leave funding as an external precondition.
func (c *Chain) Funding() (*Funding, error) {
	err := c.use(func() error {
		if c.funding == nil {
			return fmt.Errorf("%w: Chain %q has no funding control", ErrCapabilityUnavailable, c.id)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return c.funding, nil
}

type Funding struct {
	controller chainimpl.EOAFunder
	chain      *Chain
}

func (f *Funding) EnsureEOABalance(ctx context.Context, address common.Address, minimum *big.Int) error {
	return f.chain.use(func() error {
		return f.controller.EnsureEOABalance(ctx, address, minimum)
	})
}

type Mining struct {
	controller chainimpl.BlockController
	chain      *Chain
}

func (m *Mining) Pause(ctx context.Context) error {
	return m.chain.use(func() error { return m.controller.PauseMining(ctx) })
}

func (m *Mining) Resume(ctx context.Context) error {
	return m.chain.use(func() error { return m.controller.ResumeMining(ctx) })
}

func (m *Mining) Mine(ctx context.Context, blocks int) error {
	return m.chain.use(func() error { return m.controller.MineBlocks(ctx, blocks) })
}

func (m *Mining) AdvanceTime(ctx context.Context, d time.Duration) error {
	return m.chain.use(func() error { return m.controller.AdvanceTime(ctx, d) })
}

func (m *Mining) WithPaused(ctx context.Context, fn func() error) error {
	return m.chain.use(func() (err error) {
		if pauseErr := m.controller.PauseMining(ctx); pauseErr != nil {
			return pauseErr
		}
		defer func() {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), capabilityCleanupTimeout)
			defer cancel()
			err = errors.Join(err, m.controller.ResumeMining(cleanupCtx))
		}()
		return fn()
	})
}

type NodeLifecycle struct {
	controller chainimpl.FaultInjector
	chain      *Chain
}

func (n *NodeLifecycle) Stop(ctx context.Context) error {
	return n.chain.use(func() error { return n.controller.StopNode(ctx) })
}

func (n *NodeLifecycle) Start(ctx context.Context) error {
	return n.chain.use(func() error { return n.controller.StartNode(ctx) })
}

func blockTiming(block time.Duration) Timing {
	return Timing{
		BlockInterval:    block,
		CompletionBudget: 20 * block,
		PollInterval:     clampPoll(block / 4),
	}
}

func clampPoll(d time.Duration) time.Duration {
	const minPoll, maxPoll = 50 * time.Millisecond, 250 * time.Millisecond
	switch {
	case d < minPoll:
		return minPoll
	case d > maxPoll:
		return maxPoll
	default:
		return d
	}
}
