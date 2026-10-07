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
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/cli/internal/service/signer"
	"github.com/cosmos/ibc/cli/internal/store"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

// DefaultTxSubmissionDelay disables optional submission pacing.
const DefaultTxSubmissionDelay = 0 * time.Second

// Default transaction policies.
const (
	DefaultPollInterval    = 3 * time.Second
	DefaultRepriceInterval = time.Minute
	DefaultRequestTimeout  = 10 * time.Second
	DefaultMaxInFlight     = 16
	DefaultMaxGas          = 10_000_000
	DefaultMaxFeePerGas    = 500_000_000_000
)

// ETHClient is the RPC surface used by the transaction manager.
type ETHClient interface {
	BlockHashByNumber(ctx context.Context, number uint64) (common.Hash, error)
	BalanceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error)
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
	SuggestGasTipCap(ctx context.Context) (*big.Int, error)
	PendingCodeAt(ctx context.Context, account common.Address) ([]byte, error)
	EstimateGas(ctx context.Context, call ethereum.CallMsg) (uint64, error)
	NonceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error)
	SendTransaction(ctx context.Context, tx *types.Transaction) error
	TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
}

// ChainOptions controls request bounds, recovery, and confirmation policy.
type ChainOptions struct {
	TxSubmissionDelay   time.Duration
	GasFeeCapMultiplier *float64
	GasTipCapMultiplier *float64
	PollInterval        time.Duration
	RepriceInterval     time.Duration
	RequestTimeout      time.Duration
	MaxInFlight         int
	MaxGas              uint64
	MaxFeePerGas        *big.Int
	// Zero waits for the finalized block tag; a positive value uses block depth.
	Confirmations uint64
}

// TxSubmitter owns nonce allocation and durable transaction recovery for one account.
type TxSubmitter struct {
	chainID   string
	eth       ETHClient
	signer    signer.Signer
	address   common.Address
	ethSigner types.Signer
	db        store.Store
	opts      ChainOptions
	logger    *slog.Logger

	lifecycle      sync.Mutex
	ctx            context.Context
	cancel         context.CancelFunc
	done           chan struct{}
	submitMu       sync.Mutex
	lastReprice    time.Time
	lastSubmission time.Time
}

type pendingTx struct {
	attempts []signedAttempt
	packets  []store.PacketKey
}

func (p *pendingTx) latest() *signedAttempt { return &p.attempts[len(p.attempts)-1] }

type signedAttempt struct {
	store.TxAttempt
	tx *types.Transaction
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
	manager, err := New(chainID, rpcClient{eth}, chainSigner, db, opts)
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
	id, ok := new(big.Int).SetString(chainID, 10)
	if !ok || id.Sign() <= 0 {
		return nil, errors.Errorf("invalid EVM chain ID %q", chainID)
	}
	if db == nil {
		return nil, errors.New("transaction journal is required")
	}
	if chainSigner.Type() != signer.ECDSA {
		return nil, errors.New("EVM transactions require an ECDSA signer")
	}
	pub, err := crypto.DecompressPubkey(chainSigner.PublicKey())
	if err != nil {
		return nil, err
	}
	if opts.PollInterval == 0 {
		opts.PollInterval = DefaultPollInterval
	}
	if opts.RepriceInterval == 0 {
		opts.RepriceInterval = DefaultRepriceInterval
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
	if opts.PollInterval <= 0 || opts.RepriceInterval <= 0 ||
		opts.RequestTimeout <= 0 ||
		opts.MaxInFlight <= 0 || opts.MaxGas < 21000 ||
		opts.MaxFeePerGas.Sign() <= 0 ||
		opts.TxSubmissionDelay < 0 {
		return nil, errors.New("invalid transaction manager limits")
	}
	c := &TxSubmitter{
		chainID:   chainID,
		eth:       eth,
		signer:    chainSigner,
		address:   crypto.PubkeyToAddress(*pub),
		ethSigner: types.LatestSignerForChainID(id),
		db:        db,
		opts:      opts,
		logger:    slog.With("module", "txsubmitter", "chainID", chainID),
	}
	c.logger = c.logger.With("wallet", c.address.Hex())
	metrics.setWallet(chainID, c.address.Hex(), eth)
	return c, nil
}

func (c *TxSubmitter) Start(ctx context.Context) error {
	c.lifecycle.Lock()
	defer c.lifecycle.Unlock()
	if c.done != nil {
		return errors.New("transaction manager already started")
	}
	c.ctx, c.cancel = context.WithCancel(ctx)
	if _, err := c.loadPending(); err != nil {
		c.cancel()
		return err
	}
	c.poll()
	c.done = make(chan struct{})
	go c.run()
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
	c.submitMu.Lock()
	defer c.submitMu.Unlock()
	if closer, ok := c.eth.(interface{ Close() }); ok {
		closer.Close()
	}
	return nil
}

func (c *TxSubmitter) Submit(ctx context.Context, intent v2.TxIntent) error {
	if len(intent.Operations) == 0 {
		return errors.New("relay submission requires operations")
	}
	for _, op := range intent.Operations {
		if op.Kind == v2.RelayKindUnknown || op.Kind != intent.Operations[0].Kind {
			return errors.New("relay submission requires operations of the same known kind")
		}
	}
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

func (c *TxSubmitter) run() {
	defer close(c.done)
	ticker := time.NewTicker(c.opts.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.poll()
		}
	}
}

func (c *TxSubmitter) submit(ctx context.Context, intent v2.TxIntent) error {
	if time.Since(c.lastSubmission) < c.opts.TxSubmissionDelay {
		return ErrBusy
	}
	mined, err := call(ctx, c, func(ctx context.Context) (uint64, error) {
		return c.eth.NonceAt(ctx, c.address, nil)
	})
	if err != nil {
		return err
	}
	nonces, err := call(ctx, c, func(ctx context.Context) ([]uint64, error) {
		return c.db.ListPendingTxNonces(ctx, c.chainID, c.address.Hex())
	})
	if err != nil {
		return err
	}
	// Journaled nonces remain owned even after a lost commit response.
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
		return ErrBusy
	}

	tx, err := c.newTx(ctx, intent, nonce)
	if err != nil {
		return err
	}
	packets := make([]store.PacketKey, len(intent.Operations))
	for i, op := range intent.Operations {
		packets[i] = store.PacketKey{
			SourceChainID:  op.SourceChainID,
			SourceClientID: op.SourceClientID,
			Sequence:       op.Sequence,
		}
	}
	p := &pendingTx{packets: packets}
	if err = c.signAndSave(ctx, tx, intent.Operations[0].Kind.String(), p); err != nil {
		return err
	}
	c.lastSubmission = time.Now()
	c.recordEvent("submitted")
	if ctx.Err() == nil {
		c.broadcast(ctx, p)
	}
	return nil
}

func call[T any](ctx context.Context, c *TxSubmitter, fn func(context.Context) (T, error)) (T, error) {
	ctx, cancel := context.WithTimeout(ctx, c.opts.RequestTimeout)
	defer cancel()
	return fn(ctx)
}

func (c *TxSubmitter) loadPending() (map[uint64]*pendingTx, error) {
	ctx, cancel := context.WithTimeout(c.ctx, c.opts.RequestTimeout)
	defer cancel()
	attempts, err := c.db.ListPendingTxAttempts(ctx, c.chainID, c.address.Hex())
	if err != nil {
		return nil, err
	}
	pending := make(map[uint64]*pendingTx)
	for _, a := range attempts {
		p := pending[a.Nonce]
		var tx types.Transaction
		if err = tx.UnmarshalBinary(a.SignedTx); err != nil {
			return nil, err
		}
		sender, err := types.Sender(c.ethSigner, &tx)
		if err != nil || sender != c.address || tx.Hash().Hex() != a.Hash || tx.Nonce() != a.Nonce {
			return nil, errors.New("invalid signed transaction in journal")
		}
		if p == nil {
			packets, err := c.db.ListTxPackets(ctx, a.ID)
			if err != nil {
				return nil, err
			}
			p = &pendingTx{packets: packets}
			pending[a.Nonce] = p
		}
		p.attempts = append(p.attempts, signedAttempt{TxAttempt: a, tx: &tx})
		metrics.startTx(c.chainID, c.address.Hex(), a.Hash)
	}

	return pending, nil
}
