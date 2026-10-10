// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/internal/store"
)

func expireAttempts(t *testing.T, c *TxSubmitter) {
	t.Helper()
	previous := c.opts.AbandonAfter
	c.opts.AbandonAfter = time.Nanosecond
	require.NoError(t, c.reconciler.abandonExpired(t.Context()))
	c.opts.AbandonAfter = previous
}

func TestReconcilerExpiresAttemptsAfterRestart(t *testing.T) {
	c, rpc, db := newManager(t)
	rpc.sendErr = errors.New("replacement transaction underpriced")
	intent := packetIntent(t, db, 1)
	require.NoError(t, c.submit(t.Context(), intent))
	first := pendingAttempts(t, c)[0]
	restarted, err := New(chainIDEth, rpc, c.signer, db, c.opts)
	require.NoError(t, err)
	restarted.opts.AbandonAfter = time.Nanosecond
	restarted.reconciler.reconcile(t.Context())
	pending, err := restarted.HasPending(t.Context(), intent.Operations[0])
	require.NoError(t, err)
	require.False(t, pending)
	require.Len(t, rpc.sent, 1)
	stored, err := db.GetTxAttempt(t.Context(), chainIDEth, first.Hash)
	require.NoError(t, err)
	require.Equal(t, store.TxAbandoned, stored.Status)
	require.Equal(t, first.SignedTx, stored.SignedTx)
	restarted.opts.AbandonAfter = DefaultAbandonAfter
	require.NoError(t, restarted.submit(t.Context(), intent))
	require.Equal(t, rpc.sent[0].Nonce(), rpc.sent[1].Nonce())
	require.Positive(t, rpc.sent[1].GasFeeCap().Cmp(rpc.sent[0].GasFeeCap()))
	require.Positive(t, rpc.sent[1].GasTipCap().Cmp(rpc.sent[0].GasTipCap()))
	pending, err = restarted.HasPending(t.Context(), intent.Operations[0])
	require.NoError(t, err)
	require.True(t, pending, "a retry starts a new abandonment window")
}

func TestPendingChecksLeaveExpiryToReconciler(t *testing.T) {
	c, rpc, db := newManager(t)
	intent := packetIntent(t, db, 1)
	require.NoError(t, c.submit(t.Context(), intent))
	c.opts.AbandonAfter = time.Nanosecond
	pending, err := c.HasPending(t.Context(), intent.Operations[0])
	require.NoError(t, err)
	require.True(t, pending)
	rpc.errorRead = errors.New("node unavailable")
	c.reconciler.reconcile(t.Context())
	pending, err = c.HasPending(t.Context(), intent.Operations[0])
	require.NoError(t, err)
	require.False(t, pending, "expiry must not depend on RPC availability")
	rpc.errorRead = nil
	require.NoError(t, c.submit(t.Context(), intent))
	require.Equal(t, rpc.sent[0].Nonce(), rpc.sent[1].Nonce())
	require.Positive(t, rpc.sent[1].GasFeeCap().Cmp(rpc.sent[0].GasFeeCap()))
}

func TestAbandonmentReleasesWorkAboveNonceGap(t *testing.T) {
	c, rpc, db := newManager(t)
	c.opts.MaxInFlight = 2
	rpc.mined = 1
	intent := packetIntent(t, db, 1)
	require.NoError(t, c.submit(t.Context(), intent))
	require.Equal(t, uint64(1), rpc.sent[0].Nonce())
	rpc.mined = 0
	expireAttempts(t, c)
	pending, err := c.HasPending(t.Context(), intent.Operations[0])
	require.NoError(t, err)
	require.False(t, pending)
	require.NoError(t, c.submit(t.Context(), intent))
	require.Len(t, rpc.sent, 2)
	require.Equal(t, uint64(0), rpc.sent[1].Nonce())
	require.Equal(t, intent.Data, rpc.sent[1].Data())
}

func TestRetryRetainsFeeHistoryAcrossAbandonment(t *testing.T) {
	c, rpc, db := newManager(t)
	rpc.sendErr = errors.New("replacement transaction underpriced")
	intent := packetIntent(t, db, 1)
	require.NoError(t, c.submit(t.Context(), intent))
	for i := 1; i <= 3; i++ {
		expireAttempts(t, c)
		restarted, err := New(chainIDEth, rpc, c.signer, db, c.opts)
		require.NoError(t, err)
		require.NoError(t, restarted.submit(t.Context(), intent))
		require.Equal(t, rpc.sent[0].Nonce(), rpc.sent[i].Nonce())
		require.Equal(t, bump(rpc.sent[i-1].GasFeeCap()), rpc.sent[i].GasFeeCap())
		require.Equal(t, bump(rpc.sent[i-1].GasTipCap()), rpc.sent[i].GasTipCap())
	}
	history, err := db.ListTxAttemptsAtNonce(t.Context(), chainIDEth, c.address.Hex(), 0)
	require.NoError(t, err)
	require.Len(t, history, 4)
}

func TestRetrySimulationFailureKeepsPreviousAttempt(t *testing.T) {
	c, rpc, db := newManager(t)
	intent := packetIntent(t, db, 1)
	require.NoError(t, c.submit(t.Context(), intent))
	expireAttempts(t, c)
	rpc.estimateErr = errors.New("execution reverted")
	require.ErrorContains(t, c.submit(t.Context(), intent), "execution reverted")
	require.Len(t, rpc.sent, 1)
	history, err := db.ListTxAttemptsAtNonce(t.Context(), chainIDEth, c.address.Hex(), 0)
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.Equal(t, store.TxAbandoned, history[0].Status)
	rpc.estimateErr = nil
	intent.Data = []byte{0xbe, 0xef}
	require.NoError(t, c.submit(t.Context(), intent))
	require.Equal(t, intent.Data, rpc.sent[1].Data())
	require.Positive(t, rpc.sent[1].GasFeeCap().Cmp(rpc.sent[0].GasFeeCap()))
}

func TestAbandonedTransactionResolvesWithoutNewSubmission(t *testing.T) {
	for _, status := range []string{store.TxConfirmed, store.TxFailed, store.TxConsumed} {
		t.Run(status, func(t *testing.T) {
			c, rpc, db := newManager(t)
			require.NoError(t, c.submit(t.Context(), packetIntent(t, db, 1)))
			first := pendingAttempts(t, c)[0]
			expireAttempts(t, c)
			rpc.mined = 1
			if status != store.TxConsumed {
				tx := rpc.sent[0]
				rpc.receipts[tx.Hash()] = &types.Receipt{
					TxHash: tx.Hash(), Status: types.ReceiptStatusSuccessful,
					BlockNumber: rpc.head.Number, BlockHash: rpc.head.Hash(),
				}
				if status == store.TxFailed {
					rpc.receipts[tx.Hash()].Status = types.ReceiptStatusFailed
				}
			}
			restarted, err := New(chainIDEth, rpc, c.signer, db, c.opts)
			require.NoError(t, err)
			restarted.reconciler.reconcile(t.Context())
			stored, err := db.GetTxAttempt(t.Context(), chainIDEth, first.Hash)
			require.NoError(t, err)
			require.Equal(t, status, stored.Status)
			require.Len(t, rpc.sent, 1)
		})
	}
}

func TestAbandonedAttemptWinsAgainstNewWork(t *testing.T) {
	c, rpc, db := newManager(t)
	firstIntent := packetIntent(t, db, 1)
	require.NoError(t, c.submit(t.Context(), firstIntent))
	first := pendingAttempts(t, c)[0]
	expireAttempts(t, c)
	secondIntent := packetIntent(t, db, 2)
	secondIntent.Data = []byte{0xbe, 0xef}
	require.NoError(t, c.submit(t.Context(), secondIntent))
	require.Equal(t, rpc.sent[0].Nonce(), rpc.sent[1].Nonce())
	rpc.mined = 1
	rpc.receipts[rpc.sent[0].Hash()] = &types.Receipt{
		TxHash: rpc.sent[0].Hash(), Status: types.ReceiptStatusSuccessful,
		BlockNumber: rpc.head.Number, BlockHash: rpc.head.Hash(),
	}
	c.reconciler.reconcile(t.Context())
	winner, err := db.GetTxAttempt(t.Context(), chainIDEth, first.Hash)
	require.NoError(t, err)
	require.Equal(t, store.TxConfirmed, winner.Status)
	loser, err := db.GetTxAttempt(t.Context(), chainIDEth, rpc.sent[1].Hash().Hex())
	require.NoError(t, err)
	require.Equal(t, store.TxReplaced, loser.Status)
	pending, err := c.HasPending(t.Context(), secondIntent.Operations[0])
	require.NoError(t, err)
	require.False(t, pending)
	packets, err := db.ListPacketsBySourceTx(t.Context(), "2", "0xsend")
	require.NoError(t, err)
	for _, packet := range packets {
		require.Nil(t, packet.RecvTxHash)
	}
	require.NoError(t, c.submit(t.Context(), secondIntent))
	require.Equal(t, uint64(1), rpc.sent[2].Nonce())
}

type failedAbandonmentStore struct{ store.Store }

func (failedAbandonmentStore) AbandonTxAttemptsBefore(context.Context, string, string, time.Time) error {
	return context.DeadlineExceeded
}

func TestAbandonmentWriteFailureRetainsOwnership(t *testing.T) {
	c, rpc, db := newManager(t)
	intent := packetIntent(t, db, 1)
	require.NoError(t, c.submit(t.Context(), intent))
	c.opts.AbandonAfter = time.Nanosecond
	c.opts.MaxInFlight = 1
	c.reconciler.db = failedAbandonmentStore{Store: db}
	c.reconciler.reconcile(t.Context())
	pending, err := c.HasPending(t.Context(), intent.Operations[0])
	require.NoError(t, err)
	require.True(t, pending)
	require.ErrorIs(t, c.submit(t.Context(), intent), ErrBusy)
	require.Len(t, pendingAttempts(t, c), 1)
	require.Len(t, rpc.sent, 1)
}
