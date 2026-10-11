// SPDX-License-Identifier: Apache-2.0

// Package evm signs and broadcasts EVM transactions.
package evm

import (
	"context"
	"log/slog"
	"math/big"
	"strings"
	"sync"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/pkg/errors"

	chainsevm "github.com/cosmos/ibc/cli/internal/chains/evm"
	"github.com/cosmos/ibc/cli/internal/service/signer"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

// Default transaction limits.
const (
	DefaultResubmitAfter  = 15 * time.Minute
	DefaultRequestTimeout = 10 * time.Second
	DefaultMaxInFlight    = 16
	DefaultMaxGas         = 10_000_000
	DefaultMaxFeePerGas   = 500000000000
	maxBroadcastAttempts  = 8
)

// ChainOptions controls transaction pricing and nonce recovery.
type ChainOptions struct {
	GasFeeCapMultiplier *float64
	GasTipCapMultiplier *float64
	ResubmitAfter       time.Duration
	RequestTimeout      time.Duration
	MaxInFlight         int
	MaxGas              uint64
	MaxFeePerGas        *big.Int
}

// ETHClient is the RPC surface used for submission and wallet metrics.
type ETHClient interface {
	BalanceAt(context.Context, common.Address, *big.Int) (*big.Int, error)
	HeaderByNumber(context.Context, *big.Int) (*types.Header, error)
	SuggestGasTipCap(context.Context) (*big.Int, error)
	PendingCodeAt(context.Context, common.Address) ([]byte, error)
	EstimateGasAtBlock(context.Context, ethereum.CallMsg, *big.Int) (uint64, error)
	NonceAt(context.Context, common.Address, *big.Int) (uint64, error)
	PendingNonceAt(context.Context, common.Address) (uint64, error)
	SendTransaction(context.Context, *types.Transaction) error
	TransactionReceipt(context.Context, common.Hash) (*types.Receipt, error)
}

// TxSubmitter serializes submissions from one dedicated account. Packet retries
// supply fresh work when the account's lowest unmined nonce stalls.
type TxSubmitter struct {
	chainID      string
	eth          ETHClient
	signer       signer.Signer
	address      common.Address
	ethSigner    types.Signer
	opts         ChainOptions
	mu           sync.Mutex
	nextNonce    uint64
	pendingNonce uint64
	pendingSince time.Time
	feeCap       *big.Int
	tipCap       *big.Int
}

// ErrBusy leaves the packet available for a later dispatch.
var ErrBusy = errors.New("transaction submission capacity reached")

func NewFromRPC(chainID, rpcURL string, chainSigner signer.Signer, opts ChainOptions) (*TxSubmitter, error) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultRequestTimeout)
	defer cancel()
	eth, err := chainsevm.DialContext(ctx, chainID, rpcURL)
	if err != nil {
		return nil, err
	}
	return New(chainID, eth, chainSigner, opts)
}

func New(chainID string, eth ETHClient, chainSigner signer.Signer, opts ChainOptions) (*TxSubmitter, error) {
	if opts.ResubmitAfter == 0 {
		opts.ResubmitAfter = DefaultResubmitAfter
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
	if opts.ResubmitAfter <= 0 || opts.RequestTimeout <= 0 || opts.MaxInFlight <= 0 || opts.MaxGas < 21000 ||
		opts.MaxFeePerGas.Sign() <= 0 {
		return nil, errors.New("invalid transaction limits")
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
		chainID: chainID, eth: eth, signer: chainSigner,
		address: crypto.PubkeyToAddress(*pub), ethSigner: types.LatestSignerForChainID(id), opts: opts,
	}
	metrics.setWallet(chainID, c.address.Hex(), eth)
	return c, nil
}

func (c *TxSubmitter) Submit(ctx context.Context, intent v2.TxIntent) (*v2.Submission, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, c.opts.RequestTimeout)
	defer cancel()
	nonce, err := c.selectNonce(ctx, time.Now())
	if err != nil {
		return nil, err
	}
	fee, tip, err := estimateFees(ctx, c.eth, &c.opts)
	if err != nil {
		return nil, err
	}
	if nonce == c.pendingNonce && c.feeCap != nil {
		fee, tip = maxBig(fee, bump(c.feeCap)), maxBig(tip, bump(c.tipCap))
	}
	var submission *v2.Submission
	for range maxBroadcastAttempts {
		tx, err := c.buildAndSignTransaction(ctx, intent.To, intent.Data, nonce, fee, tip)
		if err != nil {
			return submission, err
		}
		// A lost response may still mean the node accepted this nonce.
		c.nextNonce = max(c.nextNonce, nonce+1)
		if nonce == c.pendingNonce {
			c.feeCap, c.tipCap = fee, tip
			if c.pendingSince.IsZero() {
				c.pendingSince = time.Now()
			}
		}
		submission = &v2.Submission{
			TxHash:         tx.Hash().Hex(),
			SubmittedAt:    time.Now().UTC(),
			RelayerAddress: c.address.Hex(),
		}
		err = broadcast(ctx, c.eth, tx)
		if err == nil {
			if nonce == c.pendingNonce {
				c.pendingSince = time.Now()
			}
			metrics.startTx(c.chainID, c.address.Hex(), tx.Hash().Hex())
			slog.Debug("Transaction broadcast", "chainID", c.chainID, "nonce", nonce, "txHash", tx.Hash())
			return submission, nil
		}
		if !strings.Contains(strings.ToLower(err.Error()), "underpriced") {
			return submission, errors.Wrapf(err, "broadcasting transaction %s at nonce %d", tx.Hash(), nonce)
		}
		fee, tip = bump(fee), bump(tip)
	}
	return submission, errors.Errorf(
		"replacement still underpriced after %d attempts at nonce %d",
		maxBroadcastAttempts,
		nonce,
	)
}

func (c *TxSubmitter) selectNonce(ctx context.Context, now time.Time) (uint64, error) {
	mined, err := c.eth.NonceAt(ctx, c.address, nil)
	if err != nil {
		return 0, err
	}
	pending, err := c.eth.PendingNonceAt(ctx, c.address)
	if err != nil {
		return 0, err
	}
	if mined != c.pendingNonce {
		c.pendingNonce, c.pendingSince = mined, time.Time{}
		c.feeCap, c.tipCap = nil, nil
	}
	c.nextNonce = max(c.nextNonce, mined, pending)
	if c.nextNonce > mined {
		if c.pendingSince.IsZero() {
			c.pendingSince = now
		}
		if now.Sub(c.pendingSince) >= c.opts.ResubmitAfter {
			return mined, nil
		}
	}
	if c.nextNonce-mined >= uint64(c.opts.MaxInFlight) {
		return 0, ErrBusy
	}
	return c.nextNonce, nil
}

// ShouldRetry leaves missing transactions pending until the retry interval expires.
func (c *TxSubmitter) ShouldRetry(ctx context.Context, txHash string, sentAt time.Time) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, c.opts.RequestTimeout)
	defer cancel()
	receipt, err := c.eth.TransactionReceipt(ctx, common.HexToHash(txHash))
	switch {
	case errors.Is(err, ethereum.NotFound):
		if time.Since(sentAt) >= c.opts.ResubmitAfter {
			metrics.forgetTx(c.chainID, txHash)
			return true, nil
		}
		return false, v2.ErrTxNotFound
	case err != nil:
		return false, errors.Wrapf(err, "getting receipt for tx %s", txHash)
	default:
		metrics.endTx(c.chainID, receipt)
		return receipt.Status != types.ReceiptStatusSuccessful, nil
	}
}
