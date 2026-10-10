// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"log/slog"
	"maps"
	"slices"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/cli/internal/store"
)

type transactionReconciler struct {
	chainID string
	eth     ETHClient
	address common.Address
	db      store.Store
	opts    *ChainOptions
	logger  *slog.Logger
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
	if err := r.abandonExpired(ctx); err != nil {
		r.logger.Warn("Abandoning expired transactions", "error", err)
		return
	}
	state, err := r.latestMinedState(ctx)
	if err != nil {
		r.logger.Warn("Reading mined account state", "error", err)
		return
	}
	if err = r.reopenReorgedTransactions(ctx, state); err != nil {
		r.logger.Warn("Reopening unmined transactions", "error", err)
		return
	}
	pending, err := r.loadUnresolvedTransactions(ctx, state.nonce)
	if err != nil {
		r.logger.Warn("Loading transaction journal", "error", err)
		return
	}
	for _, nonce := range slices.Sorted(maps.Keys(pending)) {
		if nonce >= state.nonce {
			continue
		}
		if err := r.resolveTransactionState(ctx, pending[nonce], state); err != nil {
			r.logger.Warn("Resolving transaction nonce", "nonce", nonce, "error", err)
		}
	}
}

func (r *transactionReconciler) abandonExpired(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, r.opts.RequestTimeout)
	defer cancel()
	return r.db.AbandonTxAttemptsBefore(ctx, r.chainID, r.address.Hex(), time.Now().Add(-r.opts.AbandonAfter))
}

func (r *transactionReconciler) reopenReorgedTransactions(ctx context.Context, state minedState) error {
	requestCtx, cancel := context.WithTimeout(ctx, r.opts.RequestTimeout)
	resolutions, err := r.db.ListTxResolutionsFrom(requestCtx, r.chainID, r.address.Hex(), state.nonce)
	cancel()
	if err != nil {
		return err
	}
	for _, resolution := range resolutions {
		if state.blockNumber < resolution.BlockNumber {
			continue
		}
		requestCtx, cancel := context.WithTimeout(ctx, r.opts.RequestTimeout)
		err := r.db.ReopenTxAttempts(requestCtx, r.chainID, r.address.Hex(), resolution)
		cancel()
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *transactionReconciler) resolveTransactionState(
	ctx context.Context,
	attempts []store.TxAttempt,
	state minedState,
) error {
	ctx, cancel := context.WithTimeout(ctx, r.opts.RequestTimeout)
	defer cancel()
	hashes := make([]common.Hash, len(attempts))
	for i, a := range attempts {
		hashes[i] = common.HexToHash(a.Hash)
	}
	receipt, err := r.minedReceipt(ctx, hashes, state)
	if err != nil {
		return err
	}
	if err := r.checkCanonical(ctx, state); err != nil {
		return err
	}
	outcome := store.TxOutcome{
		ChainID: r.chainID, Sender: r.address.Hex(), Nonce: attempts[0].Nonce, Status: store.TxConsumed,
		BlockNumber: state.blockNumber, BlockHash: state.blockHash.Hex(),
	}
	if receipt != nil {
		outcome.Hash = receipt.TxHash.Hex()
		outcome.Status = store.TxConfirmed
		if receipt.Status == types.ReceiptStatusFailed {
			outcome.Status = store.TxFailed
		}
	}
	if err := r.db.ResolveTxNonce(ctx, outcome); err != nil {
		return err
	}
	if receipt != nil {
		metrics.startTx(r.chainID, r.address.Hex(), receipt.TxHash.Hex())
		metrics.endTx(r.chainID, receipt)
	}
	r.logger.Info("Transaction nonce resolved", "nonce", outcome.Nonce, "status", outcome.Status, "hash", outcome.Hash)
	return nil
}

type minedState struct {
	blockNumber uint64
	blockHash   common.Hash
	nonce       uint64
}

func (r *transactionReconciler) latestMinedState(ctx context.Context) (minedState, error) {
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

func (r *transactionReconciler) loadUnresolvedTransactions(
	ctx context.Context,
	minedNonce uint64,
) (map[uint64][]store.TxAttempt, error) {
	ctx, cancel := context.WithTimeout(ctx, r.opts.RequestTimeout)
	defer cancel()
	attempts, err := r.db.ListUnresolvedTxAttempts(ctx, r.chainID, r.address.Hex(), minedNonce)
	if err != nil {
		return nil, err
	}
	pending := make(map[uint64][]store.TxAttempt)
	for _, a := range attempts {
		pending[a.Nonce] = append(pending[a.Nonce], a)
	}
	return pending, nil
}
