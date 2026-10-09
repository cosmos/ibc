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
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/internal/service/signer"
	"github.com/cosmos/ibc/cli/internal/store"
)

type countingSigner struct {
	signer.Signer
	calls int
	err   error
}

func (s *countingSigner) Sign(ctx context.Context, digest []byte) ([]byte, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.Signer.Sign(ctx, digest)
}

type reconciliationStoreStub struct {
	store.Store
	attempts   []store.TxAttempt
	saveErr    error
	saved      []store.TxAttempt
	outcomes   []store.TxOutcome
	resolved   []store.TxResolution
	reopened   []store.TxResolution
	broadcasts map[string]time.Time
}

func (s *reconciliationStoreStub) ListUnresolvedTxAttempts(
	context.Context,
	string,
	string,
	uint64,
) ([]store.TxAttempt, error) {
	return s.attempts, nil
}

func (s *reconciliationStoreStub) Transact(_ context.Context, f func(store.Repository) error) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	return f(s)
}

func (s *reconciliationStoreStub) InsertTxAttempt(_ context.Context, a store.TxAttempt) (int64, error) {
	a.ID = int64(len(s.attempts) + 1)
	s.saved = append(s.saved, a)
	s.attempts = append(s.attempts, a)
	return a.ID, nil
}

func (s *reconciliationStoreStub) ListTxOperations(context.Context, int64) ([]store.TxOperation, error) {
	return nil, nil
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

func (s *reconciliationStoreStub) SetTxBroadcast(_ context.Context, _, hash string, at time.Time) error {
	s.broadcasts[hash] = at
	for i := range s.attempts {
		if s.attempts[i].Hash == hash {
			s.attempts[i].LastBroadcastAt = &at
		}
	}
	return nil
}

func newTestReconciler(t *testing.T) (*transactionReconciler, *testRPC, *reconciliationStoreStub, *time.Time) {
	t.Helper()
	now := time.Unix(1000, 0).UTC()
	key, err := signer.GenerateLocalSecp256k1Signer()
	require.NoError(t, err)
	pub, err := crypto.DecompressPubkey(key.PublicKey())
	require.NoError(t, err)
	rpc := newRPC(t)
	db := &reconciliationStoreStub{broadcasts: make(map[string]time.Time)}
	r := &transactionReconciler{
		chainID: chainIDEth, eth: rpc, signer: &countingSigner{Signer: key},
		address: crypto.PubkeyToAddress(*pub), ethSigner: types.LatestSignerForChainID(big.NewInt(1)),
		db: db, logger: slog.Default(), now: func() time.Time { return now },
		opts: &ChainOptions{
			PollInterval: time.Second, RepriceInterval: time.Minute, RequestTimeout: time.Second,
			AbandonAfter: DefaultAbandonAfter,
			MaxGas:       DefaultMaxGas, MaxFeePerGas: big.NewInt(DefaultMaxFeePerGas),
		},
	}
	return r, rpc, db, &now
}

func pendingAttempt(t *testing.T, r *transactionReconciler, nonce uint64, submittedAt time.Time) *pendingTx {
	t.Helper()
	unsigned := types.NewTx(&types.DynamicFeeTx{
		ChainID: r.ethSigner.ChainID(), Nonce: nonce, Gas: 21000,
		GasFeeCap: big.NewInt(100), GasTipCap: big.NewInt(10),
	})
	tx, err := signTransaction(t.Context(), unsigned, r.signer.(*countingSigner).Signer, r.ethSigner, r.address, r.opts)
	require.NoError(t, err)
	raw, err := tx.MarshalBinary()
	require.NoError(t, err)
	return &pendingTx{attempts: []signedAttempt{{
		tx: tx,
		TxAttempt: store.TxAttempt{
			ChainID: r.chainID, Sender: r.address.Hex(), Hash: tx.Hash().Hex(), Nonce: nonce,
			SubmittedAt: submittedAt, SignedTx: raw,
		},
	}}}
}

func TestReconcilerBroadcastsBeforeRepricing(t *testing.T) {
	r, rpc, db, now := newTestReconciler(t)
	key := r.signer.(*countingSigner)
	first := pendingAttempt(t, r, 0, now.Add(-2*time.Minute))
	second := pendingAttempt(t, r, 1, now.Add(-2*time.Minute))
	db.attempts = []store.TxAttempt{first.latest().TxAttempt, second.latest().TxAttempt}
	original := first.latest().tx
	later := second.latest().tx
	rpc.beforeSend = func(tx *types.Transaction) {
		for _, a := range db.attempts {
			if a.Hash == tx.Hash().Hex() {
				return
			}
		}
		t.Fatal("broadcast before persistence")
	}

	r.reconcile(t.Context())
	require.Len(t, rpc.sent, 2)
	require.Equal(t, original.Hash(), rpc.sent[0].Hash())
	require.Equal(t, later.Hash(), rpc.sent[1].Hash())
	require.Zero(t, key.calls)
	require.Equal(t, *now, db.broadcasts[original.Hash().Hex()])

	*now = now.Add(r.opts.PollInterval / 2)
	r.reconcile(t.Context())
	require.Equal(t, 1, key.calls)
	require.Len(t, db.saved, 1)
	require.Equal(t, db.saved[0].Hash, rpc.sent[2].Hash().Hex())
	require.Equal(t, uint64(0), db.saved[0].Nonce)
	require.Equal(t, *now, db.broadcasts[db.saved[0].Hash])

	r.reconcile(t.Context())
	require.Len(t, rpc.sent, 3)
	require.Equal(t, 1, key.calls)

	*now = now.Add(r.opts.PollInterval)
	r.reconcile(t.Context())
	require.Len(t, rpc.sent, 5)
	require.Equal(t, rpc.sent[2].Hash(), rpc.sent[3].Hash())
	require.Equal(t, later.Hash(), rpc.sent[4].Hash())
	require.Equal(t, 1, key.calls)
}

func TestReconcilerBacksOffFailedReplacement(t *testing.T) {
	for _, failure := range []string{"signing", "persistence"} {
		t.Run(failure, func(t *testing.T) {
			r, rpc, db, now := newTestReconciler(t)
			key := r.signer.(*countingSigner)
			p := pendingAttempt(t, r, 0, now.Add(-2*time.Minute))
			broadcastAt := *now
			p.latest().LastBroadcastAt = &broadcastAt
			db.attempts = []store.TxAttempt{p.latest().TxAttempt}
			if failure == "signing" {
				key.err = context.DeadlineExceeded
			} else {
				db.saveErr = context.DeadlineExceeded
			}

			r.reconcile(t.Context())
			require.Equal(t, 1, key.calls)
			require.Empty(t, db.saved)
			require.Empty(t, rpc.sent)

			key.err, db.saveErr = nil, nil
			*now = now.Add(r.opts.RepriceInterval - time.Nanosecond)
			r.reconcile(t.Context())
			require.Equal(t, 1, key.calls)
			require.Len(t, rpc.sent, 1)
			require.Equal(t, p.latest().tx.Hash(), rpc.sent[0].Hash())

			*now = now.Add(time.Nanosecond)
			r.reconcile(t.Context())
			require.Equal(t, 2, key.calls)
			require.Len(t, db.saved, 1)
			require.Equal(t, db.saved[0].Hash, rpc.sent[1].Hash().Hex())
		})
	}
}

func TestReconcilerResolvesMinedNonce(t *testing.T) {
	for _, status := range []string{store.TxConfirmed, store.TxFailed, store.TxConsumed} {
		t.Run(status, func(t *testing.T) {
			r, rpc, db, now := newTestReconciler(t)
			key := r.signer.(*countingSigner)
			p := pendingAttempt(t, r, 0, *now)
			db.attempts = []store.TxAttempt{p.latest().TxAttempt}
			rpc.mined = 1
			hash := ""
			if status != store.TxConsumed {
				hash = p.latest().Hash
				rpc.receipts[p.latest().tx.Hash()] = &types.Receipt{
					TxHash: p.latest().tx.Hash(), Status: types.ReceiptStatusSuccessful,
					BlockNumber: rpc.head.Number, BlockHash: rpc.head.Hash(),
				}
				if status == store.TxFailed {
					rpc.receipts[p.latest().tx.Hash()].Status = types.ReceiptStatusFailed
				}
			}

			r.reconcile(t.Context())
			require.Equal(t, []store.TxOutcome{{
				ChainID: r.chainID, Sender: r.address.Hex(), Nonce: 0, Hash: hash, Status: status,
				BlockNumber: rpc.head.Number.Uint64(), BlockHash: rpc.head.Hash().Hex(),
			}}, db.outcomes)
			require.Empty(t, rpc.sent)
			require.Zero(t, key.calls)
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
			r, rpc, db, _ := newTestReconciler(t)
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
	r, rpc, _, _ := newTestReconciler(t)
	rpc.reportedHash = common.HexToHash("0xbe5a")
	rpc.mined = 7
	state, err := r.latestMinedState(t.Context())
	require.NoError(t, err)
	require.Equal(t, minedState{blockNumber: 10, blockHash: rpc.reportedHash, nonce: 7}, state)
	require.Equal(t, &rpc.reportedHash, rpc.nonceBlock.BlockHash)
	require.Nil(t, rpc.nonceBlock.BlockNumber)
	require.True(t, rpc.nonceBlock.RequireCanonical)
}
