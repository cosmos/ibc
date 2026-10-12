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
	// maxConnsPerNode bounds open connections to one node; callers beyond it
	// queue for a connection instead of piling more onto the container
	// runtime's port forwarding.
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

// attempt bounds a single request to a quarter of w.Timeout, so a stalled
// request is given up and repeated before the wait runs out.
func (w TransactionWait) attempt(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, w.Timeout/4)
}

// Retry calls f until it succeeds, fails definitively, w.Timeout passes, or
// ctx ends, and returns its last result; a w that is not positive fails at
// once. Each attempt is bounded by a quarter of w.Timeout, so a stalled
// request is retried on another connection instead of using up the wait, and
// attempts are spaced by w.PollInterval. Only a node's answer is definitive,
// except an internal error: a transport failure, a timed-out attempt, a null
// where a value must exist (ethereum.NotFound included), JSON-RPC -32603, and
// HTTP 5xx and 429 are retried.
func Retry[T any](ctx context.Context, w TransactionWait, f func(context.Context) (T, error)) (T, error) {
	var v T
	ctx, cancel, err := w.context(ctx)
	if err != nil {
		return v, err
	}
	defer cancel()
	for {
		attempt, cancel := w.attempt(ctx)
		v, err = f(attempt)
		cancel()
		if !transient(err) {
			return v, err
		}
		select {
		case <-ctx.Done():
			return v, err
		case <-time.After(w.PollInterval):
		}
	}
}

// jsonRPCInternalError is the JSON-RPC 2.0 internal error, the node's
// equivalent of HTTP 500. Besu answered it to both Chains' gas estimates at
// once in a sustained load run.
const jsonRPCInternalError = -32603

// transient reports whether a request is worth repeating: the node gave no
// answer (a transport failure or a timed-out attempt), answered null where a
// value must exist, as Besu briefly can for the latest block, failed
// internally, or answered with an HTTP status that asks to come back (5xx,
// 429). Any other error the node returned, such as a revert, and any other
// HTTP status are definitive.
func transient(err error) bool {
	var (
		nodeErr rpc.Error
		httpErr rpc.HTTPError
	)
	switch {
	case err == nil:
		return false
	case errors.As(err, &nodeErr):
		return nodeErr.ErrorCode() == jsonRPCInternalError
	case errors.As(err, &httpErr):
		return httpErr.StatusCode >= http.StatusInternalServerError || httpErr.StatusCode == http.StatusTooManyRequests
	default:
		return true
	}
}

// nodeTransport is shared by every Dial, so connections are pooled per node
// rather than per client: geth's Close does not release a client's
// transport. Every open connection may stay idle for reuse; the default of
// two idle connections makes many concurrent callers (the load test's
// senders) dial and close a connection per request, which leaves the host's
// local ports in TIME_WAIT until new connections fail.
var nodeTransport = func() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 0
	transport.MaxIdleConnsPerHost = maxConnsPerNode
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

	accountsMu sync.Mutex
	accounts   map[common.Address]*accountState
}

// accountState is this client's send state for one account.
type accountState struct {
	// gate serializes local sends from the account across nonce reads and
	// submission; sends from different accounts proceed concurrently.
	gate chan struct{}
	// floor is one past the highest nonce whose transaction this client saw
	// included. A node can briefly report an older pending nonce right after
	// the inclusion (Besu once did); the floor keeps it from being reused.
	floor uint64
}

// The caller remains responsible for readiness.
func NewConnectedClient(ctx context.Context, c *ethclient.Client) (*EVMClient, error) {
	id, err := c.ChainID(ctx)
	if err != nil {
		return nil, fmt.Errorf("query chain id: %w", err)
	}
	return &EVMClient{
		client:   c,
		chainID:  id,
		accounts: map[common.Address]*accountState{},
	}, nil
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
	// Included, so its nonce is spent, even if it reverted.
	e.raiseNonceFloor(from.addr, signed.Nonce()+1)
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
	gate := e.account(from.addr).gate
	if err := acquireTx(ctx, gate); err != nil {
		return nil, err
	}
	defer func() { <-gate }()

	signed, err := e.buildSignedTx(ctx, wait, from, to, data, value)
	if err != nil {
		return nil, err
	}
	if err := e.send(ctx, wait, signed); err != nil {
		return nil, fmt.Errorf("send tx with nonce %d (nonce floor %d): %w",
			signed.Nonce(), e.nonceFloor(from.addr), err)
	}
	return signed, nil
}

// send submits signed until the node answers. Resending the same bytes is
// safe. Once an attempt went unanswered, the node may already hold the
// transaction, and then refuses a resend ("already known", or "nonce too
// low" once it is mined); a refusal is accepted, and left to the receipt
// wait, only if the node answers that it knows the transaction.
func (e *EVMClient) send(ctx context.Context, wait TransactionWait, signed *types.Transaction) error {
	unanswered := false
	_, err := Retry(ctx, wait, func(ctx context.Context) (struct{}, error) {
		err := e.client.SendTransaction(ctx, signed)
		unanswered = unanswered || transient(err)
		return struct{}{}, err
	})
	if err == nil || !unanswered || transient(err) {
		return err
	}
	known, lookupErr := Retry(ctx, wait, func(ctx context.Context) (bool, error) {
		_, _, lookupErr := e.client.TransactionByHash(ctx, signed.Hash())
		if errors.Is(lookupErr, ethereum.NotFound) {
			// The node answered that it lacks the transaction.
			return false, nil
		}
		return lookupErr == nil, lookupErr
	})
	if lookupErr == nil && known {
		return nil
	}
	return err
}

func (e *EVMClient) account(address common.Address) *accountState {
	e.accountsMu.Lock()
	defer e.accountsMu.Unlock()
	state, ok := e.accounts[address]
	if !ok {
		state = &accountState{gate: make(chan struct{}, 1)}
		e.accounts[address] = state
	}
	return state
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
	nonce, err := Retry(ctx, wait, func(ctx context.Context) (uint64, error) {
		return e.client.PendingNonceAt(ctx, from.addr)
	})
	if err != nil {
		return nil, fmt.Errorf("pending nonce: %w", err)
	}
	nonce = max(nonce, e.nonceFloor(from.addr))

	val := new(big.Int)
	if value != nil {
		val.Set(value)
	}

	msg := ethereum.CallMsg{From: from.addr, To: to, Value: val, Data: data}
	est, err := Retry(ctx, wait, func(ctx context.Context) (uint64, error) {
		// Pending first: its timestamp is the next block's, and state stamped
		// with a block's time (the IFT rate limiter's bucket, which every
		// sender shares) can be newer than the latest block a node estimates
		// against, which reverts with an underflow. Besu, whose pending
		// estimate uses a future timestamp, can reject an otherwise valid
		// short-lived transaction there or fail internally on it under load,
		// so any node error is retried once against latest.
		est, estimateErr := e.client.EstimateGas(ctx, msg)
		var nodeErr rpc.Error
		if errors.As(estimateErr, &nodeErr) {
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

	tipCap, err := Retry(ctx, wait, e.client.SuggestGasTipCap)
	if err != nil {
		return nil, fmt.Errorf("suggest gas tip cap: %w", err)
	}
	if tipCap == nil || tipCap.Sign() <= 0 {
		tipCap = big.NewInt(fallbackTipCapWei)
	}
	head, err := Retry(ctx, wait, func(ctx context.Context) (*types.Header, error) {
		return e.client.HeaderByNumber(ctx, nil)
	})
	if err != nil {
		return nil, fmt.Errorf("latest header: %w", err)
	}
	feeCap := new(big.Int).Set(tipCap)
	if head.BaseFee != nil && head.BaseFee.Sign() > 0 {
		feeCap.Add(new(big.Int).Mul(head.BaseFee, big.NewInt(2)), tipCap)
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

func (e *EVMClient) nonceFloor(address common.Address) uint64 {
	state := e.account(address)
	e.accountsMu.Lock()
	defer e.accountsMu.Unlock()
	return state.floor
}

// raiseNonceFloor only raises: receipts of one account's transactions can
// arrive out of order.
func (e *EVMClient) raiseNonceFloor(address common.Address, floor uint64) {
	state := e.account(address)
	e.accountsMu.Lock()
	defer e.accountsMu.Unlock()
	state.floor = max(state.floor, floor)
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

// waitForReceipt polls with its own policy rather than Retry's: it repeats
// any failed poll, a node's error included, because nodes answer receipt
// queries with errors that pass, such as geth's "transaction indexing is in
// progress".
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
		attempt, cancel := wait.attempt(ctx)
		defer cancel()
		r, err := e.client.TransactionReceipt(attempt, hash)
		switch {
		case err == nil && r != nil:
			receipt = r
			return true, nil
		case errors.Is(err, ethereum.NotFound):
			// The node answered; an earlier failure no longer explains a timeout.
			pollErr = nil
		case err != nil && ctx.Err() == nil:
			// A poll cut short by the wait's own end explains nothing.
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
