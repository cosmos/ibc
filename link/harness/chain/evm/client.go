package evm

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/cosmos/ibc/link/harness/internal/poll"

	ethereum "github.com/ethereum/go-ethereum"
)

const (
	// Gas estimation undershoots for state-touching calls, so pad the estimate: +20% proportional and
	// a fixed cushion, with a 21000 minimum floor. A failed estimate is surfaced as an error (it
	// usually means the tx would revert), never silently defaulted.
	defaultGasLimit   = 21_000
	gasPaddingPercent = 20
	gasPaddingFixed   = 10_000
	// fallbackTipCapWei (1 gwei) is used when the node suggests no priority fee, which Anvil can do
	// on an empty mempool.
	fallbackTipCapWei = 1_000_000_000

	// Receipt polling: Anvil auto-mines on each tx, so receipts land almost immediately; the budget
	// is generous only to tolerate a momentarily busy node. This is condition polling, not a sleep.
	receiptPollInterval = 100 * time.Millisecond
	// ReceiptTimeout bounds receipt waits.
	ReceiptTimeout = 30 * time.Second
)

// faucetFundingWei is how much a freshly created account is funded with (100 ETH) — far more than
// any test needs, so funding is never the cause of a failure.
var faucetFundingWei = new(big.Int).Mul(big.NewInt(100), big.NewInt(params.Ether))

// EVMClient wraps an ethclient.Client with the funding/broadcast/query operations the harness needs.
// It is provider-agnostic — provider chains embed it — and holds the faucet used to fund new accounts.
//
//nolint:revive // Deliberate stutter: the family prefix keeps call sites greppable (evm.EVMClient, like anvil.Chain).
type EVMClient struct {
	client  *ethclient.Client
	chainID *big.Int
	faucet  Account

	// faucetMu serializes faucet-originated sends: every new account is funded from the single
	// faucet, so concurrent funding would read the same pending nonce and collide. Non-faucet
	// senders are the caller's responsibility to serialize.
	faucetMu sync.Mutex
}

// NewConnectedClient wraps an already-dialed ethclient, querying its chain id so the wrapper can
// sign with the correct signer. The caller owns dialing (and readiness) so Anvil startup can reuse
// the same connection it probed with.
func NewConnectedClient(ctx context.Context, c *ethclient.Client) (*EVMClient, error) {
	id, err := c.ChainID(ctx)
	if err != nil {
		return nil, fmt.Errorf("query chain id: %w", err)
	}
	return &EVMClient{client: c, chainID: id, faucet: FaucetAccount()}, nil
}

// NewVerifiedClient wraps an already-dialed ethclient as an EVMClient bound to the dev faucet and verifies
// the node reports wantChainID, closing the client on any failure. label names the chain in errors. It is
// the connect-then-verify step the anvil, external, and sandbox providers run after dialing; Besu verifies
// the chain id inside its own readiness wait and uses NewConnectedClient directly.
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

// EVM returns the client itself — the ClientProvider capability hook, promoted into every provider chain
// that embeds *EVMClient so the harness reaches a chain's concrete EVM view through one method.
func (e *EVMClient) EVM() *EVMClient { return e }

// ChainID returns the numeric EVM chain id this client signs for.
func (e *EVMClient) ChainID() *big.Int { return new(big.Int).Set(e.chainID) }

// RPCClient exposes the low-level JSON-RPC client for Anvil cheat methods ethclient does not wrap.
func (e *EVMClient) RPCClient() *rpc.Client { return e.client.Client() }

func (e *EVMClient) Close() { e.client.Close() }

func (e *EVMClient) Height(ctx context.Context) (uint64, error) {
	return e.client.BlockNumber(ctx)
}

// Tail returns the last maxBytes of s (all of it when shorter) — the tail of a container's startup log the
// EVM providers attach to a readiness-failure error.
func Tail(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	return s[len(s)-maxBytes:]
}

func (e *EVMClient) Logs(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
	return e.client.FilterLogs(ctx, q)
}

// NewFundedAccount generates a fresh key and funds it from the faucet, returning only once the
// funding tx is mined so the account is immediately usable.
func (e *EVMClient) NewFundedAccount(ctx context.Context) (Account, error) {
	acct, err := NewAccount()
	if err != nil {
		return Account{}, err
	}
	if _, err := e.BroadcastTx(ctx, e.faucet, &acct.Addr, nil, faucetFundingWei); err != nil {
		return Account{}, fmt.Errorf("fund new account %s: %w", acct.Addr, err)
	}
	return acct, nil
}

// NewReceiver mints a fresh funded account and returns its canonical hex address. Funding is deliberate:
// an EVM receiver may need gas for a follow-up action.
func (e *EVMClient) NewReceiver(ctx context.Context) (string, error) {
	acct, err := e.NewFundedAccount(ctx)
	if err != nil {
		return "", err
	}
	return acct.Addr.Hex(), nil
}

// BroadcastTx builds, signs and sends an EIP-1559 transaction from `from`, then waits for the
// receipt. A `to` of nil is contract creation with `data` as init code. It returns an error if the
// transaction reverts on-chain.
func (e *EVMClient) BroadcastTx(
	ctx context.Context,
	from Account,
	to *common.Address,
	data []byte,
	value *big.Int,
) (*types.Receipt, error) {
	signed, err := e.signAndSend(ctx, from, to, data, value)
	if err != nil {
		return nil, err
	}
	receipt, err := e.WaitForReceipt(ctx, signed.Hash())
	if err != nil {
		return nil, err
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return nil, fmt.Errorf("tx %s reverted on-chain (status %d)", signed.Hash(), receipt.Status)
	}
	return receipt, nil
}

// signAndSend builds, signs and submits the tx. Faucet-originated sends are serialized across the
// nonce read and submission: once SendTransaction returns, the tx is in the mempool and reflected by
// PendingNonceAt, so the lock can be released before the (slower) receipt wait.
func (e *EVMClient) signAndSend(
	ctx context.Context,
	from Account,
	to *common.Address,
	data []byte,
	value *big.Int,
) (*types.Transaction, error) {
	if from.Addr == e.faucet.Addr {
		e.faucetMu.Lock()
		defer e.faucetMu.Unlock()
	}
	signed, err := e.buildSignedTx(ctx, from, to, data, value)
	if err != nil {
		return nil, err
	}
	if err := e.client.SendTransaction(ctx, signed); err != nil {
		return nil, fmt.Errorf("send tx: %w", err)
	}
	return signed, nil
}

// buildSignedTx assembles a signed EIP-1559 DynamicFeeTx: pending nonce, padded gas estimate, and a
// fee cap of 2*baseFee + tip (so it stays valid across a base-fee bump).
func (e *EVMClient) buildSignedTx(
	ctx context.Context,
	from Account,
	to *common.Address,
	data []byte,
	value *big.Int,
) (*types.Transaction, error) {
	nonce, err := e.client.PendingNonceAt(ctx, from.Addr)
	if err != nil {
		return nil, fmt.Errorf("pending nonce: %w", err)
	}

	val := new(big.Int)
	if value != nil {
		val.Set(value)
	}

	est, err := e.client.EstimateGas(ctx, ethereum.CallMsg{From: from.Addr, To: to, Value: val, Data: data})
	if err != nil {
		// A failed estimate almost always means the tx would revert on-chain. Surfacing it is correct:
		// a 21000 fallback is below the intrinsic gas of a contract creation (to == nil) and would
		// only mask the real failure with a confusing one.
		return nil, fmt.Errorf("estimate gas: %w", err)
	}
	gas := est + est*gasPaddingPercent/100 + gasPaddingFixed
	if gas < defaultGasLimit {
		gas = defaultGasLimit // 21000 minimum floor
	}

	tipCap, err := e.client.SuggestGasTipCap(ctx)
	if err != nil || tipCap == nil || tipCap.Sign() <= 0 {
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
	return signTx(tx, from.Key, e.chainID)
}

// WaitNextPendingTx waits until the chain's mempool grows by at least one transaction — the
// control-plane probe block-control tests use between submitting under paused mining and asserting a
// pending state. Mempool admission is independent of block cadence (a tx enters at submit speed, not
// seal speed), so it polls at the client's own control-plane cadence, like the receipt wait below.
func (e *EVMClient) WaitNextPendingTx(ctx context.Context) error {
	before, err := e.client.PendingTransactionCount(ctx)
	if err != nil {
		return err
	}
	if err := poll.Until(ctx, receiptPollInterval, ReceiptTimeout, func(ctx context.Context) (bool, error) {
		n, err := e.client.PendingTransactionCount(ctx)
		if err != nil {
			return false, err
		}
		return n > before, nil
	}); err != nil {
		return fmt.Errorf("waiting for pending tx count to exceed %d: %w", before, err)
	}
	return nil
}

// WaitForReceipt polls for the receipt of hash until it is mined or the budget elapses. NotFound is
// the expected pending state; any other error is surfaced.
func (e *EVMClient) WaitForReceipt(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
	var receipt *types.Receipt
	err := poll.Until(ctx, receiptPollInterval, ReceiptTimeout, func(ctx context.Context) (bool, error) {
		r, err := e.client.TransactionReceipt(ctx, hash)
		if err == nil && r != nil {
			receipt = r
			return true, nil
		}
		if err != nil && !errors.Is(err, ethereum.NotFound) {
			return false, err // a real RPC error (not the expected pending NotFound) aborts the poll
		}
		return false, nil // NotFound: still pending, keep polling
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return nil, fmt.Errorf("receipt for %s not found in time: %w", hash, err)
		}
		return nil, fmt.Errorf("poll receipt %s: %w", hash, err)
	}
	return receipt, nil
}
