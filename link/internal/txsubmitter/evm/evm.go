// Package evm submits transactions to EVM chains.
package evm

import (
	"context"
	"log/slog"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/service/signer"

	v2 "github.com/cosmos/ibc/link/internal/types/v2"
	ethereum "github.com/ethereum/go-ethereum"
)

// DefaultTxSubmissionDelay the minimum delay between submissions on one chain
// when no override is configured.
const DefaultTxSubmissionDelay = 2 * time.Second

// retryExpiry is how long a submitted relay tx may sit without landing
// before ShouldRetry reports it should be cleared and resubmitted.
const retryExpiry = 2 * time.Minute

// ETHClient go-ethereum methods used by the EVM tx submitter.
type ETHClient interface {
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
	SuggestGasTipCap(ctx context.Context) (*big.Int, error)
	PendingCodeAt(ctx context.Context, account common.Address) ([]byte, error)
	EstimateGas(ctx context.Context, call ethereum.CallMsg) (uint64, error)
	PendingNonceAt(ctx context.Context, account common.Address) (uint64, error)
	SendTransaction(ctx context.Context, tx *types.Transaction) error
	TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
}

// TxSubmitter signs and broadcasts transactions on one EVM chain.
type TxSubmitter struct {
	chainID   string
	eth       ETHClient
	signer    signer.Signer
	address   common.Address
	ethSigner types.Signer

	delay      time.Duration
	feeCapMult *float64
	tipCapMult *float64

	// submissions are serialized and rate limited per chain
	mu             sync.Mutex
	lastSubmission time.Time

	logger *slog.Logger
}

// ChainOptions per-chain submission settings.
type ChainOptions struct {
	TxSubmissionDelay   time.Duration
	GasFeeCapMultiplier *float64
	GasTipCapMultiplier *float64
}

// NewFromRPC dials the chain's RPC and builds its tx submitter.
func NewFromRPC(chainID, rpcURL string, chainSigner signer.Signer, opts ChainOptions) (*TxSubmitter, error) {
	eth, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, errors.Wrapf(err, "dialing rpc for chain %q", chainID)
	}

	return New(chainID, eth, chainSigner, opts)
}

func New(chainID string, eth ETHClient, chainSigner signer.Signer, opts ChainOptions) (*TxSubmitter, error) {
	chainIDInt, ok := new(big.Int).SetString(chainID, 10)
	if !ok {
		return nil, errors.Errorf("invalid evm chain id %q", chainID)
	}

	if chainSigner.Type() != signer.ECDSA {
		return nil, errors.Errorf("signer for chain %q must be %s, got %s", chainID, signer.ECDSA, chainSigner.Type())
	}

	pub, err := crypto.DecompressPubkey(chainSigner.PublicKey())
	if err != nil {
		return nil, errors.Wrapf(err, "decompressing signer public key for chain %q", chainID)
	}

	delay := opts.TxSubmissionDelay
	if delay == 0 {
		delay = DefaultTxSubmissionDelay
	}

	return &TxSubmitter{
		chainID:    chainID,
		eth:        eth,
		signer:     chainSigner,
		address:    crypto.PubkeyToAddress(*pub),
		ethSigner:  types.LatestSignerForChainID(chainIDInt),
		delay:      delay,
		feeCapMult: opts.GasFeeCapMultiplier,
		tipCapMult: opts.GasTipCapMultiplier,
		logger:     slog.With("module", "txsubmitter", "chainID", chainID),
	}, nil
}

func (c *TxSubmitter) Submit(ctx context.Context, intent v2.TxIntent) (*v2.Submission, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if wait := c.delay - time.Since(c.lastSubmission); wait > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}

	tx, err := c.newTx(ctx, intent)
	if err != nil {
		return nil, errors.Wrap(err, "creating tx")
	}

	signature, err := c.signer.Sign(ctx, c.ethSigner.Hash(tx).Bytes())
	if err != nil {
		return nil, errors.Wrapf(err, "signing tx with address %s", c.address)
	}

	signedTx, err := tx.WithSignature(c.ethSigner, signature)
	if err != nil {
		return nil, errors.Wrap(err, "attaching signature")
	}

	if err := c.eth.SendTransaction(ctx, signedTx); err != nil {
		return nil, errors.Wrapf(err, "sending tx %s", signedTx.Hash())
	}

	c.lastSubmission = time.Now()
	c.logger.Info("Submitted tx", "txHash", signedTx.Hash(), "to", intent.To)

	return &v2.Submission{
		TxHash:         signedTx.Hash().String(),
		SubmittedAt:    time.Now().UTC(),
		RelayerAddress: c.address.String(),
	}, nil
}

func (c *TxSubmitter) newTx(ctx context.Context, intent v2.TxIntent) (*types.Transaction, error) {
	if !common.IsHexAddress(intent.To) {
		return nil, errors.Errorf("invalid to address %q", intent.To)
	}

	to := common.HexToAddress(intent.To)

	head, err := c.eth.HeaderByNumber(ctx, nil)
	if err != nil {
		return nil, errors.Wrap(err, "getting latest header")
	}

	if head.BaseFee == nil {
		return nil, errors.Errorf("chain %s has no base fee; it must support EIP-1559", c.chainID)
	}

	gasTipCap, err := c.eth.SuggestGasTipCap(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "getting suggested gas tip cap")
	}

	gasFeeCap := new(big.Int).Add(gasTipCap, new(big.Int).Mul(head.BaseFee, big.NewInt(2)))

	code, err := c.eth.PendingCodeAt(ctx, to)
	if err != nil {
		return nil, errors.Wrapf(err, "getting code at %s", to)
	}

	if len(code) == 0 {
		return nil, errors.Errorf("no contract code at %s", to)
	}

	gasLimit, err := c.eth.EstimateGas(ctx, ethereum.CallMsg{From: c.address, To: &to, Data: intent.Data})
	if err != nil {
		return nil, errors.Wrap(err, "estimating gas")
	}

	nonce, err := c.eth.PendingNonceAt(ctx, c.address)
	if err != nil {
		return nil, errors.Wrapf(err, "getting pending nonce for %s", c.address)
	}

	return types.NewTx(&types.DynamicFeeTx{
		To:        &to,
		Nonce:     nonce,
		GasFeeCap: applyMultiplier(gasFeeCap, c.feeCapMult),
		GasTipCap: applyMultiplier(gasTipCap, c.tipCapMult),
		Gas:       gasLimit,
		Data:      intent.Data,
	}), nil
}

func (c *TxSubmitter) ShouldRetry(ctx context.Context, txHash string, sentAt time.Time) (bool, error) {
	receipt, err := c.eth.TransactionReceipt(ctx, common.HexToHash(txHash))
	switch {
	case errors.Is(err, ethereum.NotFound):
		latest, errHeader := c.eth.HeaderByNumber(ctx, nil)
		if errHeader != nil {
			return false, errors.Wrap(errHeader, "getting latest header")
		}

		expiresAt := sentAt.UTC().Add(retryExpiry)
		if expiresAt.Before(time.Unix(int64(latest.Time), 0)) {
			return true, nil
		}

		return false, v2.ErrTxNotFound
	case err != nil:
		return false, errors.Wrapf(err, "getting receipt for tx %s", txHash)
	case receipt.Status != types.ReceiptStatusSuccessful:
		return true, nil
	default:
		return false, nil
	}
}

func applyMultiplier(value *big.Int, multiplier *float64) *big.Int {
	if multiplier == nil {
		return value
	}

	adjusted, _ := new(big.Float).Mul(new(big.Float).SetInt(value), big.NewFloat(*multiplier)).Int(nil)

	return adjusted
}
