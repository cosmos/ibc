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

func TestAbandonmentSurvivesRepricingAndRestart(t *testing.T) {
	c, rpc, db := newManager(t)
	rpc.sendErr = errors.New("tx fee exceeds configured cap")
	intent := packetIntent(t, db, 1)
	require.NoError(t, c.submit(t.Context(), intent))
	first := pendingAttempts(t, c)[0]
	deadline := first.SubmittedAt.Add(c.opts.AbandonAfter)
	c.reconciler.now = func() time.Time { return deadline.Add(-time.Nanosecond) }
	c.reconciler.reconcile(t.Context())
	attempts := pendingAttempts(t, c)
	require.Len(t, attempts, 2)
	require.Len(t, rpc.sent, 2)

	restarted, err := New(chainIDEth, rpc, c.signer, db, c.opts)
	require.NoError(t, err)
	restarted.reconciler.now = func() time.Time { return deadline }
	restarted.reconciler.reconcile(t.Context())
	require.Empty(t, pendingAttempts(t, restarted))
	require.Len(t, rpc.sent, 2, "expired attempts are neither rebroadcast nor repriced")
	for _, a := range attempts {
		stored, readErr := db.GetTxAttempt(t.Context(), chainIDEth, a.Hash)
		require.NoError(t, readErr)
		require.Equal(t, store.TxAbandoned, stored.Status)
		require.Equal(t, a.SignedTx, stored.SignedTx)
	}
	pending, err := restarted.HasPending(t.Context(), intent.Operations[0])
	require.NoError(t, err)
	require.False(t, pending)
	nonce, err := restarted.selectNonce(t.Context())
	require.NoError(t, err)
	require.Zero(t, nonce)
}

func TestAbandonmentReleasesWorkAboveNonceGap(t *testing.T) {
	c, rpc, db := newManager(t)
	c.opts.MaxInFlight = 2
	rpc.mined = 1
	intent := packetIntent(t, db, 1)
	require.NoError(t, c.submit(t.Context(), intent))
	first := pendingAttempts(t, c)[0]
	require.Equal(t, uint64(1), first.Nonce)

	rpc.mined = 0
	c.reconciler.now = func() time.Time { return first.SubmittedAt.Add(c.opts.AbandonAfter) }
	c.reconciler.reconcile(t.Context())
	pending, err := c.HasPending(t.Context(), intent.Operations[0])
	require.NoError(t, err)
	require.False(t, pending, "the pipeline can retry existing work without waiting for new packets")
	require.Len(t, rpc.sent, 1, "abandonment does not submit a cancellation")

	require.NoError(t, c.submit(t.Context(), intent))
	require.Equal(t, uint64(0), rpc.sent[1].Nonce())
	require.Equal(t, intent.Data, rpc.sent[1].Data())
}

func TestAbandonedTransactionCanBeResubmittedUnchanged(t *testing.T) {
	c, rpc, db := newManager(t)
	rpc.sendErr = context.DeadlineExceeded
	intent := packetIntent(t, db, 1)
	require.NoError(t, c.submit(t.Context(), intent))
	first := pendingAttempts(t, c)[0]
	c.reconciler.now = func() time.Time { return first.SubmittedAt.Add(c.opts.AbandonAfter) }
	c.reconciler.reconcile(t.Context())

	rpc.sendErr = nil
	require.NoError(t, c.submit(t.Context(), intent))
	require.Equal(t, rpc.sent[0].Hash(), rpc.sent[1].Hash())
	attempts := pendingAttempts(t, c)
	require.Len(t, attempts, 1)
	require.Equal(t, first.ID, attempts[0].ID)
	require.True(t, attempts[0].SubmittedAt.After(first.SubmittedAt))
	pending, err := c.HasPending(t.Context(), intent.Operations[0])
	require.NoError(t, err)
	require.True(t, pending)
	c.reconciler.now = time.Now
	c.reconciler.reconcile(t.Context())
	require.Len(t, pendingAttempts(t, c), 1, "resubmission starts a new abandonment window")
}

func TestAbandonedTransactionResolvesWithoutNewSubmission(t *testing.T) {
	for _, status := range []string{store.TxConfirmed, store.TxFailed, store.TxConsumed} {
		t.Run(status, func(t *testing.T) {
			c, rpc, db := newManager(t)
			require.NoError(t, c.submit(t.Context(), packetIntent(t, db, 1)))
			first := pendingAttempts(t, c)[0]
			c.reconciler.now = func() time.Time { return first.SubmittedAt.Add(c.opts.AbandonAfter) }
			c.reconciler.reconcile(t.Context())
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
	c.reconciler.now = func() time.Time { return first.SubmittedAt.Add(c.opts.AbandonAfter) }
	c.reconciler.reconcile(t.Context())

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
		require.Nil(t, packet.RecvTxHash, "nonce resolution does not establish packet completion")
	}
	require.NoError(t, c.submit(t.Context(), secondIntent))
	require.Equal(t, uint64(1), rpc.sent[2].Nonce())
}

type failedAbandonmentStore struct{ store.Store }

func (failedAbandonmentStore) AbandonTxAttempts(context.Context, string, string, uint64) error {
	return context.DeadlineExceeded
}

func TestAbandonmentWriteFailureRetainsOwnership(t *testing.T) {
	c, rpc, db := newManager(t)
	intent := packetIntent(t, db, 1)
	require.NoError(t, c.submit(t.Context(), intent))
	first := pendingAttempts(t, c)[0]
	c.reconciler.now = func() time.Time { return first.SubmittedAt.Add(c.opts.AbandonAfter) }
	c.reconciler.db = failedAbandonmentStore{Store: db}
	c.reconciler.reconcile(t.Context())
	pending, err := c.HasPending(t.Context(), intent.Operations[0])
	require.NoError(t, err)
	require.True(t, pending)
	nonce, err := c.selectNonce(t.Context())
	require.NoError(t, err)
	require.Equal(t, uint64(1), nonce)
	require.Len(t, rpc.sent, 1)

	c.reconciler.db = db
	c.reconciler.reconcile(t.Context())
	require.Empty(t, pendingAttempts(t, c))
}
