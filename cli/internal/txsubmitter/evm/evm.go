// SPDX-License-Identifier: Apache-2.0

// Package evm manages durable EVM transaction attempts.
package evm

import (
	"context"
	"log/slog"
	"math/big"
	"sync"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/cli/internal/service/signer"
	"github.com/cosmos/ibc/cli/internal/store"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

// Default transaction policies.
const (
	DefaultPollInterval   = 3 * time.Second
	DefaultAbandonAfter   = 15 * time.Minute
	DefaultRequestTimeout = 10 * time.Second
	DefaultMaxInFlight    = 16
	DefaultMaxGas         = 10_000_000
	DefaultMaxFeePerGas   = 500_000_000_000
)

// ChainOptions controls request bounds and recovery.
type ChainOptions struct {
	GasFeeCapMultiplier *float64
	GasTipCapMultiplier *float64
	PollInterval        time.Duration
	AbandonAfter        time.Duration
	RequestTimeout      time.Duration
	MaxInFlight         int
	MaxGas              uint64
	MaxFeePerGas        *big.Int
}

// ETHClient is the RPC surface used by the transaction manager.
type ETHClient interface {
	Client() *rpc.Client
	BalanceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error)
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
	SuggestGasTipCap(ctx context.Context) (*big.Int, error)
	PendingCodeAt(ctx context.Context, account common.Address) ([]byte, error)
	EstimateGasAtBlock(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) (uint64, error)
	NonceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error)
	SendTransaction(ctx context.Context, tx *types.Transaction) error
	TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
}

// TxSubmitter owns nonce allocation and durable transaction recovery for one account.
type TxSubmitter struct {
	chainID    string
	eth        ETHClient
	signer     signer.Signer
	address    common.Address
	ethSigner  types.Signer
	db         store.Store
	reconciler *transactionReconciler
	opts       ChainOptions

	lifecycle sync.Mutex
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	submitMu  sync.Mutex
}

// ErrBusy leaves unsigned work in the packet store for a later dispatch.
var ErrBusy = errors.New("transaction manager capacity reached")

func NewFromRPC(
	chainID, rpcURL string,
	chainSigner signer.Signer,
	db store.Store,
	opts ChainOptions,
) (*TxSubmitter, error) {
	timeout := opts.RequestTimeout
	if timeout == 0 {
		timeout = DefaultRequestTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	eth, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		return nil, err
	}
	manager, err := New(chainID, eth, chainSigner, db, opts)
	if err != nil {
		eth.Close()
	}
	return manager, err
}

func New(
	chainID string,
	eth ETHClient,
	chainSigner signer.Signer,
	db store.Store,
	opts ChainOptions,
) (*TxSubmitter, error) {
	if db == nil {
		return nil, errors.New("db is required")
	}
	if opts.PollInterval == 0 {
		opts.PollInterval = DefaultPollInterval
	}
	if opts.AbandonAfter == 0 {
		opts.AbandonAfter = DefaultAbandonAfter
	}
	if opts.RequestTimeout == 0 {
		opts.RequestTimeout = DefaultRequestTimeout
	}
	if opts.MaxInFlight == 0 {
		opts.MaxInFlight = DefaultMaxInFlight
	}
	if opts.MaxGas == 0 {
		opts.MaxGas = DefaultMaxGas
	}
	if opts.MaxFeePerGas == nil {
		opts.MaxFeePerGas = big.NewInt(DefaultMaxFeePerGas)
	}
	if opts.PollInterval <= 0 || opts.AbandonAfter <= 0 ||
		opts.RequestTimeout <= 0 ||
		opts.MaxInFlight <= 0 || opts.MaxGas < 21000 ||
		opts.MaxFeePerGas.Sign() <= 0 {
		return nil, errors.New("invalid limits")
	}
	id, ok := new(big.Int).SetString(chainID, 10)
	if !ok || id.Sign() <= 0 {
		return nil, errors.Errorf("invalid EVM chain ID %q", chainID)
	}
	if chainSigner.Type() != signer.ECDSA {
		return nil, errors.New("EVM transactions require an ECDSA signer")
	}
	pub, err := crypto.DecompressPubkey(chainSigner.PublicKey())
	if err != nil {
		return nil, err
	}
	c := &TxSubmitter{
		chainID: chainID, eth: eth, signer: chainSigner, address: crypto.PubkeyToAddress(*pub),
		ethSigner: types.LatestSignerForChainID(id), db: db, opts: opts,
	}
	c.reconciler = &transactionReconciler{
		chainID: chainID, eth: eth, address: c.address,
		db: db, opts: &c.opts,
		logger: slog.With("module", "txsubmitter", "chainID", chainID, "wallet", c.address.Hex()),
	}
	metrics.setWallet(chainID, c.address.Hex(), eth)
	return c, nil
}

func (c *TxSubmitter) Start() error {
	c.lifecycle.Lock()
	defer c.lifecycle.Unlock()
	if c.done != nil {
		return errors.New("transaction manager already started")
	}
	c.ctx, c.cancel = context.WithCancel(context.Background())
	c.done = make(chan struct{})
	go func() {
		defer close(c.done)
		c.reconciler.run(c.ctx)
	}()
	return nil
}

func (c *TxSubmitter) Stop() error {
	c.lifecycle.Lock()
	if c.cancel != nil {
		c.cancel()
	}
	done := c.done
	c.lifecycle.Unlock()
	if done != nil {
		<-done
	}
	return nil
}

func (c *TxSubmitter) Submit(ctx context.Context, intent v2.TxIntent) error {
	c.lifecycle.Lock()
	running, serviceCtx := c.done != nil, c.ctx
	c.lifecycle.Unlock()
	if !running {
		return errors.New("transaction manager is not running")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(serviceCtx, cancel)
	defer stop()
	c.submitMu.Lock()
	defer c.submitMu.Unlock()
	if err := serviceCtx.Err(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return c.submit(ctx, intent)
}

func (c *TxSubmitter) submit(ctx context.Context, intent v2.TxIntent) error {
	nonce, err := c.selectNonce(ctx)
	if err != nil {
		return err
	}

	tx, err := c.buildAndSignTransaction(ctx, intent.To, intent.Data, nonce)
	if err != nil {
		return err
	}
	operations := make([]store.TxOperation, len(intent.Operations))
	for i, op := range intent.Operations {
		operations[i] = store.TxOperation{
			PacketKey: store.PacketKey{
				SourceChainID: op.SourceChainID, SourceClientID: op.SourceClientID, Sequence: op.Sequence,
			},
			Type: op.Type.String(),
		}
	}
	saveCtx, cancel := context.WithTimeout(ctx, c.opts.RequestTimeout)
	defer cancel()
	if err = saveAttempt(saveCtx, c.db, tx, store.TxAttempt{
		ChainID: c.chainID, Sender: c.address.Hex(),
		SubmittedAt: time.Now().UTC(),
	}, operations); err != nil {
		return err
	}
	if ctx.Err() == nil {
		if err := broadcast(ctx, c.eth, tx, c.opts.RequestTimeout); err != nil {
			slog.With("module", "txsubmitter", "chainID", c.chainID, "wallet", c.address.Hex()).Warn(
				"Transaction broadcast failed; retaining signed attempt",
				"txHash",
				tx.Hash().Hex(),
				"nonce",
				nonce,
				"error",
				err,
			)
		}
	}
	return nil
}

func (c *TxSubmitter) selectNonce(ctx context.Context) (uint64, error) {
	requestCtx, cancel := context.WithTimeout(ctx, c.opts.RequestTimeout)
	mined, err := c.eth.NonceAt(requestCtx, c.address, nil)
	cancel()
	if err != nil {
		return 0, err
	}
	requestCtx, cancel = context.WithTimeout(ctx, c.opts.RequestTimeout)
	nonces, err := c.db.ListTxNoncesFrom(requestCtx, c.chainID, c.address.Hex(), mined)
	cancel()
	if err != nil {
		return 0, err
	}
	nonce := mined
	for _, owned := range nonces {
		if owned > nonce {
			break
		}
		if owned == nonce {
			nonce++
		}
	}
	if nonce-mined >= uint64(c.opts.MaxInFlight) {
		return 0, ErrBusy
	}

	return nonce, nil
}

func (c *TxSubmitter) HasPending(ctx context.Context, op v2.OperationMetadata) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, c.opts.RequestTimeout)
	defer cancel()
	return c.db.HasPendingOperation(ctx, store.TxOperation{
		PacketKey: store.PacketKey{
			SourceChainID: op.SourceChainID, SourceClientID: op.SourceClientID, Sequence: op.Sequence,
		},
		Type: op.Type.String(),
	})
}
