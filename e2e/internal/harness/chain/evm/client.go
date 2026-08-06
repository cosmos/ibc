package evm

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"os"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm/poll"
)

const (
	defaultGasLimit   = 21_000
	gasPaddingPercent = 20
	gasPaddingFixed   = 10_000
	// Anvil may suggest no priority fee on an empty mempool.
	fallbackTipCapWei = 1_000_000_000
)

// TransactionWait bounds transaction submission and observation. Both fields
// must be positive; PollInterval controls pending-pool and receipt probes.
type TransactionWait struct {
	Timeout      time.Duration
	PollInterval time.Duration
}

func (w TransactionWait) context(ctx context.Context) (context.Context, context.CancelFunc, error) {
	switch {
	case w.Timeout <= 0:
		return nil, nil, fmt.Errorf("evm transaction wait timeout must be greater than zero")
	case w.PollInterval <= 0:
		return nil, nil, fmt.Errorf("evm transaction wait poll interval must be greater than zero")
	default:
		waitCtx, cancel := context.WithTimeout(ctx, w.Timeout)
		return waitCtx, cancel, nil
	}
}

type EVMClient struct {
	client  *ethclient.Client
	chainID *big.Int

	// Local sends must be serialized across nonce reads and submission,
	// regardless of which account pays for the transaction.
	txGate chan struct{}
}

// The caller remains responsible for readiness.
func NewConnectedClient(ctx context.Context, c *ethclient.Client) (*EVMClient, error) {
	id, err := c.ChainID(ctx)
	if err != nil {
		return nil, fmt.Errorf("query chain id: %w", err)
	}
	return &EVMClient{client: c, chainID: id, txGate: make(chan struct{}, 1)}, nil
}

// It closes c on failure.
func NewVerifiedClient(
	ctx context.Context,
	c *ethclient.Client,
	wantChainID uint64,
	label string,
) (*EVMClient, error) {
	ec, err := NewConnectedClient(ctx, c)
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("connect %s: %w", label, err)
	}
	if got := ec.ChainID().Uint64(); got != wantChainID {
		ec.Close()
		return nil, fmt.Errorf("%s reports chain id %d, want %d", label, got, wantChainID)
	}
	return ec, nil
}

func (e *EVMClient) Client() *ethclient.Client { return e.client }

func (e *EVMClient) WithEVMClient(use func(*EVMClient) error) error { return use(e) }

func (e *EVMClient) ChainID() *big.Int { return new(big.Int).Set(e.chainID) }

func (e *EVMClient) RPCClient() *rpc.Client { return e.client.Client() }

func (e *EVMClient) Close() { e.client.Close() }

func (e *EVMClient) Height(ctx context.Context) (uint64, error) {
	return e.client.BlockNumber(ctx)
}

func Tail(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	return s[len(s)-maxBytes:]
}

func (e *EVMClient) Logs(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
	return e.client.FilterLogs(ctx, q)
}

// RequireEOA rejects addresses that cannot be funded consistently through a
// plain value transfer across managed EVM providers.
func (e *EVMClient) RequireEOA(ctx context.Context, address common.Address) error {
	if address == (common.Address{}) {
		return fmt.Errorf("EOA address is zero")
	}
	code, err := e.client.CodeAt(ctx, address, nil)
	if err != nil {
		return fmt.Errorf("query code at %s: %w", address, err)
	}
	if len(code) != 0 {
		return fmt.Errorf("address %s has contract code and is not an EOA", address)
	}
	return nil
}

// It returns only after mining and treats reverts as errors.
func (e *EVMClient) BroadcastTx(
	ctx context.Context,
	wait TransactionWait,
	from Account,
	to *common.Address,
	data []byte,
	value *big.Int,
) (*types.Receipt, error) {
	waitCtx, cancel, err := wait.context(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()

	signed, err := e.signAndSend(waitCtx, from, to, data, value)
	if err != nil {
		return nil, normalizeWaitError(waitCtx, err)
	}
	receipt, err := e.waitForReceipt(waitCtx, signed.Hash(), wait)
	if err != nil {
		return nil, err
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return nil, fmt.Errorf("tx %s reverted on-chain (status %d)", signed.Hash(), receipt.Status)
	}
	return receipt, nil
}

func (e *EVMClient) signAndSend(
	ctx context.Context,
	from Account,
	to *common.Address,
	data []byte,
	value *big.Int,
) (*types.Transaction, error) {
	if err := e.acquireTx(ctx); err != nil {
		return nil, err
	}
	defer func() { <-e.txGate }()

	signed, err := e.buildSignedTx(ctx, from, to, data, value)
	if err != nil {
		return nil, err
	}
	if err := e.client.SendTransaction(ctx, signed); err != nil {
		return nil, fmt.Errorf("send tx: %w", err)
	}
	return signed, nil
}

func (e *EVMClient) acquireTx(ctx context.Context) error {
	select {
	case e.txGate <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-e.txGate
			return fmt.Errorf("wait for transaction submission slot: %w", err)
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for transaction submission slot: %w", ctx.Err())
	}
}

func (e *EVMClient) buildSignedTx(
	ctx context.Context,
	from Account,
	to *common.Address,
	data []byte,
	value *big.Int,
) (*types.Transaction, error) {
	nonce, err := e.client.PendingNonceAt(ctx, from.addr)
	if err != nil {
		return nil, fmt.Errorf("pending nonce: %w", err)
	}

	val := new(big.Int)
	if value != nil {
		val.Set(value)
	}

	est, err := e.client.EstimateGas(ctx, ethereum.CallMsg{From: from.addr, To: to, Value: val, Data: data})
	if err != nil {
		return nil, fmt.Errorf("estimate gas: %w", err)
	}
	gas := est + est*gasPaddingPercent/100 + gasPaddingFixed
	if gas < defaultGasLimit {
		gas = defaultGasLimit
	}

	tipCap, err := e.client.SuggestGasTipCap(ctx)
	if err != nil {
		return nil, fmt.Errorf("suggest gas tip cap: %w", err)
	}
	if tipCap == nil || tipCap.Sign() <= 0 {
		tipCap = big.NewInt(fallbackTipCapWei)
	}

	hdr, err := e.client.HeaderByNumber(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("latest header: %w", err)
	}
	feeCap := new(big.Int).Set(tipCap)
	if hdr.BaseFee != nil && hdr.BaseFee.Sign() > 0 {
		feeCap.Add(new(big.Int).Mul(hdr.BaseFee, big.NewInt(2)), tipCap)
	}

	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID:   e.chainID,
		Nonce:     nonce,
		To:        to,
		Value:     val,
		Gas:       gas,
		GasFeeCap: feeCap,
		GasTipCap: tipCap,
		Data:      data,
	})
	signed, err := types.SignTx(tx, types.LatestSignerForChainID(e.chainID), from.key)
	if err != nil {
		return nil, fmt.Errorf("sign tx: %w", err)
	}
	return signed, nil
}

func (e *EVMClient) WaitNextPendingTx(ctx context.Context, wait TransactionWait) error {
	waitCtx, cancel, err := wait.context(ctx)
	if err != nil {
		return err
	}
	defer cancel()

	before, err := e.client.PendingTransactionCount(waitCtx)
	if err != nil {
		return normalizeWaitError(
			waitCtx,
			fmt.Errorf("read initial pending transaction count: %w", err),
		)
	}
	if err := poll.Until(waitCtx, wait.PollInterval, func(ctx context.Context) (bool, error) {
		n, err := e.client.PendingTransactionCount(ctx)
		if err != nil {
			return false, err
		}
		return n > before, nil
	}); err != nil {
		err = normalizeWaitError(waitCtx, err)
		return fmt.Errorf("waiting for pending tx count to exceed %d: %w", before, err)
	}
	return nil
}

func (e *EVMClient) waitForReceipt(
	ctx context.Context,
	hash common.Hash,
	wait TransactionWait,
) (*types.Receipt, error) {
	var receipt *types.Receipt
	err := poll.Until(ctx, wait.PollInterval, func(ctx context.Context) (bool, error) {
		r, err := e.client.TransactionReceipt(ctx, hash)
		if err == nil && r != nil {
			receipt = r
			return true, nil
		}
		if err != nil && !errors.Is(err, ethereum.NotFound) {
			return false, err
		}
		return false, nil
	})
	if err != nil {
		err = normalizeWaitError(ctx, err)
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return nil, fmt.Errorf("receipt for %s not found in time: %w", hash, err)
		}
		return nil, fmt.Errorf("poll receipt %s: %w", hash, err)
	}
	return receipt, nil
}

// go-ethereum can surface a context deadline as the underlying connection's
// os.ErrDeadlineExceeded before context.Err observes the timer firing.
func normalizeWaitError(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		if errors.Is(err, ctxErr) {
			return err
		}
		return fmt.Errorf("%w (%w)", ctxErr, err)
	}
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return fmt.Errorf("%w (%w)", context.DeadlineExceeded, err)
	}
	return err
}
