// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"log/slog"
	"maps"
	"math/big"
	"slices"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/cli/internal/service/signer"
	"github.com/cosmos/ibc/cli/internal/store"
)

type transactionReconciler struct {
	chainID     string
	eth         ETHClient
	signer      signer.Signer
	address     common.Address
	ethSigner   types.Signer
	db          store.Store
	opts        *ChainOptions
	logger      *slog.Logger
	now         func() time.Time
	lastReprice time.Time
}

func (r *transactionReconciler) run(ctx context.Context) {
	ticker := time.NewTicker(r.opts.PollInterval)
	defer ticker.Stop()
	for {
		r.reconcile(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *transactionReconciler) reconcile(ctx context.Context) {
	state, err := r.minedState(ctx)
	if err != nil {
		r.logger.Warn("Reading mined account state", "error", err)
		return
	}
	if err = r.reopen(ctx, state); err != nil {
		r.logger.Warn("Reopening unmined transactions", "error", err)
		return
	}
	pending, err := r.loadPending(ctx)
	if err != nil {
		r.logger.Warn("Loading transaction journal", "error", err)
		return
	}
	for _, p := range pending {
		for _, a := range p.attempts {
			metrics.startTx(r.chainID, r.address.Hex(), a.Hash)
		}
	}
	for _, nonce := range slices.Sorted(maps.Keys(pending)) {
		p := pending[nonce]
		if nonce < state.nonce {
			if err := r.resolveNonce(ctx, p, state); err != nil {
				r.logger.Warn("Resolving transaction nonce", "nonce", nonce, "error", err)
			}
			continue
		}
		r.retryPending(ctx, p, state.nonce)
	}
}

func (r *transactionReconciler) retryPending(ctx context.Context, p *pendingTx, minedNonce uint64) {
	if p.latest().LastBroadcastAt != nil && p.latest().Nonce == minedNonce &&
		r.now().Sub(p.latest().SubmittedAt) >= r.opts.RepriceInterval &&
		r.now().Sub(r.lastReprice) >= r.opts.RepriceInterval {
		if err := r.reprice(ctx, p); err != nil {
			r.logger.Warn("Transaction replacement blocked", "nonce", p.latest().Nonce, "error", err)
		}
		// Back off even when signing or persistence fails.
		r.lastReprice = r.now()
	}
	if p.latest().LastBroadcastAt == nil || r.now().Sub(*p.latest().LastBroadcastAt) >= r.opts.PollInterval {
		broadcastAttempt(ctx, ctx, r.eth, r.db, p.latest(), r.now(), r.opts.RequestTimeout)
	}
}

func (r *transactionReconciler) reopen(ctx context.Context, state minedState) error {
	resolutions, err := call(ctx, r.opts.RequestTimeout, func(ctx context.Context) ([]store.TxResolution, error) {
		return r.db.ListTxResolutionsFrom(ctx, r.chainID, r.address.Hex(), state.nonce)
	})
	if err != nil {
		return err
	}
	for _, resolution := range resolutions {
		if state.blockNumber < resolution.BlockNumber {
			continue
		}
		requestCtx, cancel := context.WithTimeout(ctx, r.opts.RequestTimeout)
		err := r.reopenNonce(requestCtx, resolution, state)
		cancel()
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *transactionReconciler) reopenNonce(
	ctx context.Context,
	resolution store.TxResolution,
	state minedState,
) error {
	hash, err := r.blockHashByNumber(ctx, resolution.BlockNumber)
	if err != nil || hash == common.HexToHash(resolution.BlockHash) {
		return err
	}
	if err := r.checkCanonical(ctx, state); err != nil {
		return err
	}
	return r.db.ReopenTxAttempts(ctx, r.chainID, r.address.Hex(), resolution)
}

func (r *transactionReconciler) resolveNonce(ctx context.Context, p *pendingTx, state minedState) error {
	ctx, cancel := context.WithTimeout(ctx, r.opts.RequestTimeout)
	defer cancel()
	receipt, err := r.minedReceipt(ctx, p.hashes(), state)
	if err != nil {
		return err
	}
	if err := r.checkCanonical(ctx, state); err != nil {
		return err
	}
	outcome := store.TxOutcome{
		ChainID: r.chainID, Sender: r.address.Hex(), Nonce: p.latest().Nonce, Status: store.TxConsumed,
		BlockNumber: state.blockNumber, BlockHash: state.blockHash.Hex(),
	}
	if receipt != nil {
		outcome.Hash = receipt.TxHash.Hex()
		outcome.Status = store.TxConfirmed
		if receipt.Status == types.ReceiptStatusFailed {
			outcome.Status = store.TxReverted
		}
	}
	if err := r.db.ResolveTxNonce(ctx, outcome); err != nil {
		return err
	}
	if receipt != nil {
		metrics.endTx(r.chainID, receipt)
	}
	for _, a := range p.attempts {
		metrics.forgetTx(r.chainID, a.Hash)
	}
	r.logger.Info("Transaction nonce resolved", "nonce", outcome.Nonce, "status", outcome.Status, "hash", outcome.Hash)
	return nil
}

func (r *transactionReconciler) reprice(ctx context.Context, p *pendingTx) error {
	signed, err := r.replacement(ctx, p.latest().tx)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, r.opts.RequestTimeout)
	defer cancel()
	return saveAttempt(ctx, r.db, signed, store.TxAttempt{
		ChainID: r.chainID, Sender: r.address.Hex(), Kind: p.latest().Kind, SubmittedAt: r.now().UTC(),
	}, p)
}

type minedState struct {
	blockNumber uint64
	blockHash   common.Hash
	nonce       uint64
}

func (r *transactionReconciler) minedState(ctx context.Context) (minedState, error) {
	ctx, cancel := context.WithTimeout(ctx, r.opts.RequestTimeout)
	defer cancel()
	head, err := r.eth.HeaderByNumber(ctx, nil)
	if err != nil {
		return minedState{}, err
	}
	if head == nil || head.Number == nil || !head.Number.IsUint64() {
		return minedState{}, errors.New("invalid mined block")
	}
	number := head.Number.Uint64()
	hash, err := r.blockHashByNumber(ctx, number)
	if err != nil {
		return minedState{}, err
	}
	var nonce hexutil.Uint64
	err = r.eth.Client().CallContext(ctx, &nonce, "eth_getTransactionCount", r.address,
		rpc.BlockNumberOrHashWithHash(hash, true))
	return minedState{blockNumber: number, blockHash: hash, nonce: uint64(nonce)}, err
}

// Use RPC-reported hashes; geth's Header.Hash() differs on Besu QBFT.
func (r *transactionReconciler) blockHashByNumber(ctx context.Context, number uint64) (common.Hash, error) {
	var block *struct {
		Hash common.Hash `json:"hash"`
	}
	if err := r.eth.Client().
		CallContext(ctx, &block, "eth_getBlockByNumber", hexutil.EncodeUint64(number), false); err != nil {
		return common.Hash{}, err
	}
	if block == nil || block.Hash == (common.Hash{}) {
		return common.Hash{}, ethereum.NotFound
	}
	return block.Hash, nil
}

func (r *transactionReconciler) checkCanonical(ctx context.Context, state minedState) error {
	hash, err := r.blockHashByNumber(ctx, state.blockNumber)
	if err != nil {
		return err
	}
	if hash != state.blockHash {
		return errors.New("mined block changed during reconciliation")
	}
	return nil
}

func (r *transactionReconciler) minedReceipt(
	ctx context.Context,
	hashes []common.Hash,
	state minedState,
) (*types.Receipt, error) {
	var lookupErr error
	for _, hash := range hashes {
		receipt, err := r.eth.TransactionReceipt(ctx, hash)
		if errors.Is(err, ethereum.NotFound) {
			continue
		}
		if err != nil {
			lookupErr = err
			continue
		}
		if receipt == nil || receipt.BlockNumber == nil || !receipt.BlockNumber.IsUint64() ||
			receipt.BlockNumber.Uint64() > state.blockNumber || receipt.TxHash != hash ||
			receipt.Status > types.ReceiptStatusSuccessful {
			lookupErr = errors.Errorf("invalid receipt for %s", hash.Hex())
			continue
		}
		canonical, err := r.blockHashByNumber(ctx, receipt.BlockNumber.Uint64())
		if err != nil {
			lookupErr = err
			continue
		}
		if canonical != receipt.BlockHash {
			lookupErr = errors.Errorf("noncanonical receipt for %s", hash.Hex())
			continue
		}
		return receipt, nil
	}
	return nil, lookupErr
}

func (r *transactionReconciler) replacement(ctx context.Context, old *types.Transaction) (*types.Transaction, error) {
	fee, tip, err := estimateFees(ctx, r.eth, r.opts)
	if err != nil {
		return nil, err
	}
	fee = maxBig(fee, bump(old.GasFeeCap()))
	tip = maxBig(tip, bump(old.GasTipCap()))
	tx := types.NewTx(
		&types.DynamicFeeTx{
			ChainID:    old.ChainId(),
			Nonce:      old.Nonce(),
			To:         old.To(),
			Value:      old.Value(),
			Data:       old.Data(),
			Gas:        old.Gas(),
			GasFeeCap:  fee,
			GasTipCap:  tip,
			AccessList: old.AccessList(),
		},
	)
	return signTransaction(ctx, tx, r.signer, r.ethSigner, r.address, r.opts)
}

func (r *transactionReconciler) decode(raw []byte) (*types.Transaction, error) {
	var tx types.Transaction
	if err := tx.UnmarshalBinary(raw); err != nil {
		return nil, err
	}
	sender, err := types.Sender(r.ethSigner, &tx)
	if err != nil || sender != r.address {
		return nil, errors.New("invalid signed transaction in journal")
	}
	return &tx, nil
}

// Round up by 20%, including when the previous tip was zero.
func bump(v *big.Int) *big.Int {
	out := new(big.Int).Mul(v, big.NewInt(120))
	out.Add(out, big.NewInt(99))
	out.Div(out, big.NewInt(100))
	return maxBig(out, new(big.Int).Add(v, big.NewInt(1)))
}

func maxBig(a, b *big.Int) *big.Int {
	if a.Cmp(b) >= 0 {
		return a
	}
	return b
}

func (r *transactionReconciler) loadPending(ctx context.Context) (map[uint64]*pendingTx, error) {
	ctx, cancel := context.WithTimeout(ctx, r.opts.RequestTimeout)
	defer cancel()
	attempts, err := r.db.ListPendingTxAttempts(ctx, r.chainID, r.address.Hex())
	if err != nil {
		return nil, err
	}
	pending := make(map[uint64]*pendingTx)
	for _, a := range attempts {
		p := pending[a.Nonce]
		tx, err := r.decode(a.SignedTx)
		if err != nil {
			return nil, err
		}
		if tx.Hash().Hex() != a.Hash || tx.Nonce() != a.Nonce {
			return nil, errors.New("invalid signed transaction in journal")
		}
		if p == nil {
			packets, err := r.db.ListTxPackets(ctx, a.ID)
			if err != nil {
				return nil, err
			}
			p = &pendingTx{packets: packets}
			pending[a.Nonce] = p
		}
		p.attempts = append(p.attempts, signedAttempt{TxAttempt: a, tx: tx})
	}

	return pending, nil
}
