// SPDX-License-Identifier: Apache-2.0

// Package evm signs and broadcasts EVM transactions.
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
	"github.com/pkg/errors"

	chainsevm "github.com/cosmos/ibc/cli/internal/chains/evm"
	"github.com/cosmos/ibc/cli/internal/service/signer"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

// Default transaction limits.
const (
	DefaultPendingTxTimeout = 2 * time.Minute
	DefaultRequestTimeout   = 10 * time.Second
	DefaultMaxInFlight      = 16
	DefaultMaxFeePerGas     = 20000000000
	maxBroadcastAttempts    = 8
)

// ChainOptions controls transaction pricing and nonce recovery.
type ChainOptions struct {
	GasFeeCapMultiplier *float64
	GasTipCapMultiplier *float64
	PendingTxTimeout    time.Duration
	RequestTimeout      time.Duration
	MaxInFlight         int
	MaxFeePerGas        *big.Int
}

// ETHClient go-ethereum methods used by the EVM tx submitter.
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

// TxSubmitter signs and broadcasts transactions on one EVM chain.
type TxSubmitter struct {
	chainID                        string
	eth                            ETHClient
	signer                         signer.Signer
	address                        common.Address
	ethSigner                      types.Signer
	opts                           ChainOptions
	mu                             sync.Mutex
	nextNonce                      uint64
	lowestUnminedNonceHeight       uint64
	lowestUnminedNonce             uint64
	lowestUnminedNoncePendingSince time.Time
	lastAttemptedFeeCap            *big.Int
	lastAttemptedTipCap            *big.Int
}

// ErrBusy leaves the packet available for a later dispatch.
var ErrBusy = errors.New("transaction submission capacity reached")

func NewFromRPC(chainID, rpcURL string, chainSigner signer.Signer, opts ChainOptions) (*TxSubmitter, error) {
	eth, err := chainsevm.Dial(chainID, rpcURL)
	if err != nil {
		return nil, err
	}
	return New(chainID, eth, chainSigner, opts)
}

func New(chainID string, eth ETHClient, chainSigner signer.Signer, opts ChainOptions) (*TxSubmitter, error) {
	if opts.PendingTxTimeout == 0 {
		opts.PendingTxTimeout = DefaultPendingTxTimeout
	}
	if opts.RequestTimeout == 0 {
		opts.RequestTimeout = DefaultRequestTimeout
	}
	if opts.MaxInFlight == 0 {
		opts.MaxInFlight = DefaultMaxInFlight
	}
	if opts.MaxFeePerGas == nil {
		opts.MaxFeePerGas = big.NewInt(DefaultMaxFeePerGas)
	}
	if opts.PendingTxTimeout <= 0 || opts.RequestTimeout <= 0 || opts.MaxInFlight <= 0 ||
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
		return nil, errors.Wrapf(err, "decompressing signer public key for chain %q", chainID)
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
	nonce, head, err := c.selectNonce(ctx, time.Now())
	if err != nil {
		return nil, err
	}
	fee, tip, err := estimateFees(ctx, c.eth, head.BaseFee, &c.opts)
	if err != nil {
		return nil, err
	}
	if nonce == c.lowestUnminedNonce && c.lastAttemptedFeeCap != nil {
		fee, tip = maxBig(fee, bump(c.lastAttemptedFeeCap)), maxBig(tip, bump(c.lastAttemptedTipCap))
		fee, tip = capFees(fee, tip, c.opts.MaxFeePerGas)
	}
	txData, err := c.prepareTransaction(ctx, intent.To, intent.Data, nonce, fee, tip)
	if err != nil {
		return nil, err
	}
	var submission *v2.Submission
	for range maxBroadcastAttempts {
		txData.GasFeeCap, txData.GasTipCap = fee, tip
		tx, err := signTransaction(ctx, types.NewTx(txData), c.signer, c.ethSigner, c.address)
		if err != nil {
			return submission, err
		}
		submission = &v2.Submission{
			TxHash:         tx.Hash().Hex(),
			SubmittedAt:    time.Now().UTC(),
			RelayerAddress: c.address.Hex(),
		}
		err = broadcast(ctx, c.eth, tx)
		if err == nil {
			c.nextNonce = max(c.nextNonce, nonce+1)
			if nonce == c.lowestUnminedNonce {
				c.lastAttemptedFeeCap, c.lastAttemptedTipCap = fee, tip
				c.lowestUnminedNoncePendingSince = time.Now()
			}
			metrics.startTx(c.chainID, c.address.Hex(), tx.Hash().Hex())
			slog.Debug("Transaction broadcast", "chainID", c.chainID, "nonce", nonce, "txHash", tx.Hash())
			return submission, nil
		}
		if !isReplacementUnderpriced(err) {
			return submission, errors.Wrapf(err, "broadcasting transaction %s at nonce %d", tx.Hash(), nonce)
		}
		if nonce == c.lowestUnminedNonce {
			c.lastAttemptedFeeCap, c.lastAttemptedTipCap = fee, tip
		}
		nextFee, nextTip := capFees(bump(fee), bump(tip), c.opts.MaxFeePerGas)
		if nextFee.Cmp(fee) == 0 && nextTip.Cmp(tip) == 0 {
			return submission, errors.Wrapf(err, "fee cap %s wei reached for nonce %d", c.opts.MaxFeePerGas, nonce)
		}
		fee, tip = nextFee, nextTip
	}
	return submission, errors.Errorf(
		"replacement still underpriced after %d attempts at nonce %d",
		maxBroadcastAttempts,
		nonce,
	)
}

func (c *TxSubmitter) selectNonce(ctx context.Context, now time.Time) (uint64, *types.Header, error) {
	head, err := c.eth.HeaderByNumber(ctx, nil)
	if err != nil {
		return 0, nil, errors.Wrap(err, "getting latest header for nonce selection")
	}
	height := head.Number.Uint64()
	if height < c.lowestUnminedNonceHeight {
		return 0, nil, errors.Errorf(
			"RPC block height %d is behind last nonce observation at %d",
			height,
			c.lowestUnminedNonceHeight,
		)
	}
	mined, err := c.eth.NonceAt(ctx, c.address, head.Number)
	if err != nil {
		return 0, nil, errors.Wrapf(err, "getting mined nonce for %s at block %s", c.address, head.Number)
	}
	pending, err := c.eth.PendingNonceAt(ctx, c.address)
	if err != nil {
		return 0, nil, errors.Wrapf(err, "getting pending nonce for %s", c.address)
	}
	c.lowestUnminedNonceHeight = height
	if mined != c.lowestUnminedNonce {
		c.lowestUnminedNonce, c.lowestUnminedNoncePendingSince = mined, time.Time{}
		c.lastAttemptedFeeCap, c.lastAttemptedTipCap = nil, nil
	}
	c.nextNonce = max(c.nextNonce, mined, pending)
	if c.nextNonce > mined {
		if c.lowestUnminedNoncePendingSince.IsZero() {
			c.lowestUnminedNoncePendingSince = now
		}
		if now.Sub(c.lowestUnminedNoncePendingSince) >= c.opts.PendingTxTimeout {
			return mined, head, nil
		}
	}
	if c.nextNonce-mined >= uint64(c.opts.MaxInFlight) {
		return 0, nil, ErrBusy
	}
	return c.nextNonce, head, nil
}

// ShouldRetry leaves missing transactions pending until the retry interval expires.
func (c *TxSubmitter) ShouldRetry(ctx context.Context, txHash string, sentAt time.Time) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, c.opts.RequestTimeout)
	defer cancel()
	receipt, err := c.eth.TransactionReceipt(ctx, common.HexToHash(txHash))
	switch {
	case errors.Is(err, ethereum.NotFound):
		if time.Since(sentAt) >= c.opts.PendingTxTimeout {
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
