// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"sync"
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
	// feeMaxAge is how long concurrent senders share one fee reading. The
	// fee cap's doubled base fee covers several blocks of base-fee growth.
	feeMaxAge = time.Second
	// maxConnsPerNode bounds open connections to one node; callers beyond it
	// queue for a connection instead of piling more onto Docker Desktop's
	// port forwarding.
	maxConnsPerNode = 128
)

// TransactionWait bounds a transaction's submission and, separately, its
// inclusion. Both fields must be positive; PollInterval spaces pending-pool
// and receipt probes and retries of failed requests, and a single request
// gets at most a quarter of Timeout.
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

// retry calls f until it succeeds, fails definitively, or ctx ends, and
// returns its last result. Each attempt gets a quarter of w.Timeout, so a
// stalled request is retried on another connection instead of using up ctx.
func retry[T any](ctx context.Context, w TransactionWait, f func(context.Context) (T, error)) (T, error) {
	for {
		attempt, cancel := context.WithTimeout(ctx, w.Timeout/4)
		v, err := f(attempt)
		cancel()
		if !transient(err) || ctx.Err() != nil {
			return v, err
		}
		select {
		case <-ctx.Done():
			return v, err
		case <-time.After(w.PollInterval):
		}
	}
}

// transient reports whether a request is worth repeating: the node gave no
// answer (a transport failure or a timed-out attempt), or answered null where
// a value must exist, as Besu briefly can for the latest block. An error the
// node returned, such as a revert, is definitive.
func transient(err error) bool {
	var nodeErr rpc.Error
	return err != nil && !errors.As(err, &nodeErr)
}

// nodeTransport is shared by every Dial, so idle connections are pooled per
// node rather than per client: geth's Close does not release a client's
// transport. It keeps up to 512 idle connections per node for reuse; the
// default of two makes many concurrent callers (the load test's senders)
// dial and close a connection per request, which exhausts Docker Desktop's
// port forwarding with "connection refused".
var nodeTransport = func() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 0
	transport.MaxIdleConnsPerHost = 512
	transport.MaxConnsPerHost = maxConnsPerNode
	return transport
}()

// Dial connects to a node's JSON-RPC endpoint, reusing HTTP connections
// across concurrent calls.
func Dial(ctx context.Context, rawURL string) (*ethclient.Client, error) {
	client, err := rpc.DialOptions(ctx, rawURL, rpc.WithHTTPClient(&http.Client{Transport: nodeTransport}))
	if err != nil {
		return nil, err
	}
	return ethclient.NewClient(client), nil
}

type EVMClient struct {
	client  *ethclient.Client
	chainID *big.Int

	// Local sends from one account are serialized across nonce reads and
	// submission; sends from different accounts proceed concurrently.
	txGatesMu sync.Mutex
	txGates   map[common.Address]chan struct{}

	// The last fee reading, shared by concurrent senders for feeMaxAge.
	feeMu      sync.Mutex
	feeRead    time.Time
	feeTip     *big.Int
	feeBaseFee *big.Int
}

// The caller remains responsible for readiness.
func NewConnectedClient(ctx context.Context, c *ethclient.Client) (*EVMClient, error) {
	id, err := c.ChainID(ctx)
	if err != nil {
		return nil, fmt.Errorf("query chain id: %w", err)
	}
	return &EVMClient{client: c, chainID: id, txGates: map[common.Address]chan struct{}{}}, nil
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

// It returns only after mining and treats reverts as errors. Submission and
// inclusion each get wait.Timeout, so a slow submission does not shorten the
// wait for inclusion.
func (e *EVMClient) BroadcastTx(
	ctx context.Context,
	wait TransactionWait,
	from Account,
	to *common.Address,
	data []byte,
	value *big.Int,
) (*types.Receipt, error) {
	signed, err := e.submit(ctx, wait, from, to, data, value)
	if err != nil {
		return nil, err
	}
	receipt, err := e.WaitForReceipt(ctx, signed.Hash(), wait)
	if err != nil {
		return nil, err
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return nil, fmt.Errorf("tx %s reverted on-chain (status %d)", signed.Hash(), receipt.Status)
	}
	return receipt, nil
}

// submit builds, signs, and sends a transaction within wait.Timeout.
func (e *EVMClient) submit(
	ctx context.Context,
	wait TransactionWait,
	from Account,
	to *common.Address,
	data []byte,
	value *big.Int,
) (*types.Transaction, error) {
	submitCtx, cancel, err := wait.context(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()
	signed, err := e.signAndSend(submitCtx, wait, from, to, data, value)
	if err != nil {
		return nil, normalizeWaitError(submitCtx, err)
	}
	return signed, nil
}

func (e *EVMClient) signAndSend(
	ctx context.Context,
	wait TransactionWait,
	from Account,
	to *common.Address,
	data []byte,
	value *big.Int,
) (*types.Transaction, error) {
	gate := e.txGate(from.addr)
	if err := acquireTx(ctx, gate); err != nil {
		return nil, err
	}
	defer func() { <-gate }()

	signed, err := e.buildSignedTx(ctx, wait, from, to, data, value)
	if err != nil {
		return nil, err
	}
	if err := e.send(ctx, wait, signed); err != nil {
		return nil, fmt.Errorf("send tx: %w", err)
	}
	return signed, nil
}

// send submits signed until the node answers. Resending the same bytes is
// safe. Once an attempt went unanswered, the node may already hold the
// transaction, so a later refusal ("already known", or "nonce too low" once
// it is mined) is left to the receipt wait to settle.
func (e *EVMClient) send(ctx context.Context, wait TransactionWait, signed *types.Transaction) error {
	unanswered := false
	_, err := retry(ctx, wait, func(ctx context.Context) (struct{}, error) {
		err := e.client.SendTransaction(ctx, signed)
		unanswered = unanswered || transient(err)
		return struct{}{}, err
	})
	if err != nil && unanswered && !transient(err) {
		return nil
	}
	return err
}

func (e *EVMClient) txGate(from common.Address) chan struct{} {
	e.txGatesMu.Lock()
	defer e.txGatesMu.Unlock()
	gate, ok := e.txGates[from]
	if !ok {
		gate = make(chan struct{}, 1)
		e.txGates[from] = gate
	}
	return gate
}

func acquireTx(ctx context.Context, gate chan struct{}) error {
	select {
	case gate <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-gate
			return fmt.Errorf("wait for transaction submission slot: %w", err)
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for transaction submission slot: %w", ctx.Err())
	}
}

func (e *EVMClient) buildSignedTx(
	ctx context.Context,
	wait TransactionWait,
	from Account,
	to *common.Address,
	data []byte,
	value *big.Int,
) (*types.Transaction, error) {
	nonce, err := retry(ctx, wait, func(ctx context.Context) (uint64, error) {
		return e.client.PendingNonceAt(ctx, from.addr)
	})
	if err != nil {
		return nil, fmt.Errorf("pending nonce: %w", err)
	}

	val := new(big.Int)
	if value != nil {
		val.Set(value)
	}

	msg := ethereum.CallMsg{From: from.addr, To: to, Value: val, Data: data}
	est, err := retry(ctx, wait, func(ctx context.Context) (uint64, error) {
		est, estimateErr := e.client.EstimateGas(ctx, msg)
		if estimateErr != nil && !transient(estimateErr) {
			// Besu can reject otherwise-valid short-lived transactions because its
			// pending estimate uses a future timestamp. Retry against latest.
			est, estimateErr = e.client.EstimateGasAtBlock(ctx, msg, nil)
		}
		return est, estimateErr
	})
	if err != nil {
		return nil, fmt.Errorf("estimate gas: %w", err)
	}
	gas := est + est*gasPaddingPercent/100 + gasPaddingFixed
	if gas < defaultGasLimit {
		gas = defaultGasLimit
	}

	tipCap, baseFee, err := e.fees(ctx, wait)
	if err != nil {
		return nil, err
	}
	feeCap := new(big.Int).Set(tipCap)
	if baseFee != nil && baseFee.Sign() > 0 {
		feeCap.Add(new(big.Int).Mul(baseFee, big.NewInt(2)), tipCap)
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

// fees returns the tip and the latest base fee (nil before EIP-1559), read at
// most once per feeMaxAge across concurrent senders.
func (e *EVMClient) fees(ctx context.Context, wait TransactionWait) (tip, baseFee *big.Int, err error) {
	e.feeMu.Lock()
	defer e.feeMu.Unlock()
	if e.feeTip == nil || time.Since(e.feeRead) >= feeMaxAge {
		tip, err := retry(ctx, wait, e.client.SuggestGasTipCap)
		if err != nil {
			return nil, nil, fmt.Errorf("suggest gas tip cap: %w", err)
		}
		if tip == nil || tip.Sign() <= 0 {
			tip = big.NewInt(fallbackTipCapWei)
		}
		header, err := retry(ctx, wait, func(ctx context.Context) (*types.Header, error) {
			return e.client.HeaderByNumber(ctx, nil)
		})
		if err != nil {
			return nil, nil, fmt.Errorf("latest header: %w", err)
		}
		e.feeRead, e.feeTip, e.feeBaseFee = time.Now(), tip, header.BaseFee
	}
	return new(big.Int).Set(e.feeTip), e.feeBaseFee, nil
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

// WaitForReceipt polls until hash is mined or wait.Timeout expires. A failed
// poll is retried at the next one.
func (e *EVMClient) WaitForReceipt(
	ctx context.Context,
	hash common.Hash,
	wait TransactionWait,
) (*types.Receipt, error) {
	waitCtx, cancel, err := wait.context(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()
	return e.waitForReceipt(waitCtx, hash, wait)
}

func (e *EVMClient) waitForReceipt(
	ctx context.Context,
	hash common.Hash,
	wait TransactionWait,
) (*types.Receipt, error) {
	var (
		receipt *types.Receipt
		pollErr error
	)
	err := poll.Until(ctx, wait.PollInterval, func(ctx context.Context) (bool, error) {
		attempt, cancel := context.WithTimeout(ctx, wait.Timeout/4)
		defer cancel()
		r, err := e.client.TransactionReceipt(attempt, hash)
		switch {
		case err == nil && r != nil:
			receipt = r
			return true, nil
		case err != nil && !errors.Is(err, ethereum.NotFound):
			pollErr = err
		}
		return false, nil
	})
	if err != nil {
		err = fmt.Errorf("receipt for %s not found in time: %w", hash, normalizeWaitError(ctx, err))
		if pollErr != nil {
			err = fmt.Errorf("%w (last poll error: %w)", err, pollErr)
		}
		return nil, err
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
