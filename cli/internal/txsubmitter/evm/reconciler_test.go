// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"log/slog"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/internal/store"
)

type reconciliationStoreStub struct {
	store.Store
	attempts []store.TxAttempt
	outcomes []store.TxOutcome
	resolved []store.TxResolution
	reopened []store.TxResolution
}

func (s *reconciliationStoreStub) AbandonTxAttemptsBefore(context.Context, string, string, time.Time) error {
	return nil
}

func (s *reconciliationStoreStub) ListUnresolvedTxAttempts(
	context.Context,
	string,
	string,
	uint64,
) ([]store.TxAttempt, error) {
	return s.attempts, nil
}

func (s *reconciliationStoreStub) ListTxResolutionsFrom(
	context.Context,
	string,
	string,
	uint64,
) ([]store.TxResolution, error) {
	return s.resolved, nil
}

func (s *reconciliationStoreStub) ReopenTxAttempts(
	_ context.Context,
	_, _ string,
	resolution store.TxResolution,
) error {
	s.reopened = append(s.reopened, resolution)
	return nil
}

func (s *reconciliationStoreStub) ResolveTxNonce(_ context.Context, outcome store.TxOutcome) error {
	s.outcomes = append(s.outcomes, outcome)
	return nil
}

func newTestReconciler(t *testing.T) (*transactionReconciler, *testRPC, *reconciliationStoreStub) {
	t.Helper()
	rpc := newRPC(t)
	db := &reconciliationStoreStub{}
	r := &transactionReconciler{
		chainID: chainIDEth, eth: rpc, address: common.HexToAddress("0x1"),
		db: db, logger: slog.Default(), opts: &ChainOptions{RequestTimeout: time.Second},
	}
	return r, rpc, db
}

func TestReconcilerDoesNotBroadcastUnminedAttempts(t *testing.T) {
	r, rpc, db := newTestReconciler(t)
	db.attempts = []store.TxAttempt{{Nonce: 0}, {Nonce: 1}}
	r.reconcile(t.Context())
	require.Empty(t, rpc.sent)
	require.Empty(t, db.outcomes)
}

func TestReconcilerResolvesMinedNonce(t *testing.T) {
	for _, status := range []string{store.TxConfirmed, store.TxFailed, store.TxConsumed} {
		t.Run(status, func(t *testing.T) {
			r, rpc, db := newTestReconciler(t)
			hash := common.HexToHash("0x1")
			db.attempts = []store.TxAttempt{{Hash: hash.Hex(), Nonce: 0}}
			rpc.mined = 1
			winner := ""
			if status != store.TxConsumed {
				winner = hash.Hex()
				rpc.receipts[hash] = &types.Receipt{
					TxHash: hash, Status: types.ReceiptStatusSuccessful,
					BlockNumber: rpc.head.Number, BlockHash: rpc.head.Hash(),
				}
				if status == store.TxFailed {
					rpc.receipts[hash].Status = types.ReceiptStatusFailed
				}
			}
			r.reconcile(t.Context())
			require.Equal(t, []store.TxOutcome{{
				ChainID: r.chainID, Sender: r.address.Hex(), Nonce: 0, Hash: winner, Status: status,
				BlockNumber: rpc.head.Number.Uint64(), BlockHash: rpc.head.Hash().Hex(),
			}}, db.outcomes)
			require.Empty(t, rpc.sent)
		})
	}
}

func TestReconcilerReopeningRequiresResolutionHeight(t *testing.T) {
	for _, tc := range []struct {
		name   string
		height uint64
		reopen bool
	}{
		{name: "older block", height: 9},
		{name: "same height", height: 10, reopen: true},
		{name: "newer block", height: 11, reopen: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, rpc, db := newTestReconciler(t)
			db.resolved = []store.TxResolution{{Nonce: 0, BlockNumber: 10, BlockHash: common.HexToHash("0x10").Hex()}}
			rpc.head.Number = new(big.Int).SetUint64(tc.height)
			r.reconcile(t.Context())
			if tc.reopen {
				require.Equal(t, db.resolved, db.reopened)
			} else {
				require.Empty(t, db.reopened)
			}
		})
	}
}

func TestReconcilerMinedStateRequiresCanonicalBlock(t *testing.T) {
	r, rpc, _ := newTestReconciler(t)
	rpc.reportedHash = common.HexToHash("0xbe5a")
	rpc.mined = 7
	state, err := r.latestMinedState(t.Context())
	require.NoError(t, err)
	require.Equal(t, minedState{blockNumber: 10, blockHash: rpc.reportedHash, nonce: 7}, state)
	require.Equal(t, &rpc.reportedHash, rpc.nonceBlock.BlockHash)
	require.Nil(t, rpc.nonceBlock.BlockNumber)
	require.True(t, rpc.nonceBlock.RequireCanonical)
}
