// SPDX-License-Identifier: Apache-2.0

package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func testTransactionJournal(t *testing.T, db Store) {
	t.Helper()
	t.Run("transactionJournal", func(t *testing.T) {
		ctx := t.Context()
		key := PacketKey{SourceChainID: "journal", SourceClientID: "source", Sequence: 1}
		require.NoError(t, db.UpsertPacket(ctx, UpsertPacket{
			Status: RelayStatusPending, SourceChainID: key.SourceChainID, DestinationChainID: "target",
			SourceTxHash: "send", SourceTxTime: time.Now().UTC(), PacketSequenceNumber: key.Sequence,
			PacketSourceClientID: key.SourceClientID, PacketDestinationClientID: "dest",
			PacketTimeoutTimestamp: time.Now().Add(time.Hour),
		}))
		a := TxAttempt{
			ChainID: "target", Sender: "sender", Hash: "first", Kind: "recv", Nonce: 7,
			SignedTx: []byte{1, 2, 3}, SubmittedAt: time.Now().UTC(),
		}
		missing := key
		missing.Sequence = 999
		_, err := SaveTxAttempt(ctx, db, a, []PacketKey{key, missing})
		require.Error(t, err)
		_, err = db.GetTxAttempt(ctx, a.ChainID, a.Hash)
		require.ErrorIs(t, err, ErrNotFound)
		pending, err := db.HasPendingPacketSubmission(ctx, key, "recv")
		require.NoError(t, err)
		require.False(t, pending, "packet associations must roll back with the attempt")

		id, err := SaveTxAttempt(ctx, db, a, []PacketKey{key})
		require.NoError(t, err)
		links, err := db.ListTxPackets(ctx, id)
		require.NoError(t, err)
		require.Equal(t, []PacketKey{key}, links)
		found, err := db.GetTxAttempt(ctx, a.ChainID, a.Hash)
		require.NoError(t, err)
		require.Equal(t, a.SignedTx, found.SignedTx)
		require.Nil(t, found.LastBroadcastAt)
		broadcast := time.Now().UTC().Truncate(time.Microsecond)
		require.NoError(t, db.SetTxBroadcast(ctx, a.ChainID, a.Hash, broadcast))
		found, err = db.GetTxAttempt(ctx, a.ChainID, a.Hash)
		require.NoError(t, err)
		require.WithinDuration(t, broadcast, *found.LastBroadcastAt, time.Microsecond)

		replacement := a
		replacement.Hash, replacement.SignedTx = "second", []byte{4, 5, 6}
		_, err = SaveTxAttempt(ctx, db, replacement, []PacketKey{key})
		require.NoError(t, err)
		next := a
		next.Hash, next.Nonce = "third", 8
		_, err = SaveTxAttempt(ctx, db, next, []PacketKey{key})
		require.NoError(t, err)
		nonces, err := db.ListPendingTxNonces(ctx, a.ChainID, a.Sender)
		require.NoError(t, err)
		require.Equal(t, []uint64{7, 8}, nonces)
		pending, err = db.HasPendingPacketSubmission(ctx, key, "recv")
		require.NoError(t, err)
		require.True(t, pending)
		pending, err = db.HasPendingPacketSubmission(ctx, key, "ack")
		require.NoError(t, err)
		require.False(t, pending)

		require.NoError(
			t,
			db.ResolveTxNonce(
				ctx,
				TxOutcome{ChainID: a.ChainID, Sender: a.Sender, Nonce: a.Nonce, Hash: a.Hash, Status: TxConfirmed},
			),
		)
		for hash, status := range map[string]string{a.Hash: TxConfirmed, replacement.Hash: TxReplaced} {
			resolved, errRead := db.GetTxAttempt(ctx, a.ChainID, hash)
			require.NoError(t, errRead)
			require.Equal(t, status, resolved.Status)
			require.NotEmpty(t, resolved.SignedTx, "history remains available")
		}
		pending, err = db.HasPendingPacketSubmission(ctx, key, "recv")
		require.NoError(t, err)
		require.True(t, pending, "another nonce still owns this operation")
		require.NoError(
			t,
			db.ResolveTxNonce(
				ctx,
				TxOutcome{ChainID: next.ChainID, Sender: next.Sender, Nonce: next.Nonce, Status: TxConsumed},
			),
		)
		pending, err = db.HasPendingPacketSubmission(ctx, key, "recv")
		require.NoError(t, err)
		require.False(t, pending)
		attempts, err := db.ListPendingTxAttempts(ctx, a.ChainID, a.Sender)
		require.NoError(t, err)
		require.Empty(t, attempts)
		packets, err := db.ListPacketsBySourceTx(ctx, key.SourceChainID, "send")
		require.NoError(t, err)
		require.Nil(t, packets[0].RecvTxHash, "resolving a nonce does not establish packet completion")
	})
	for _, status := range []string{TxConfirmed, TxReverted, TxConsumed} {
		t.Run("resolveNonce/"+status, func(t *testing.T) {
			ctx := t.Context()
			attempts := []TxAttempt{
				{ChainID: status, Sender: "sender", Hash: "winner"},
				{ChainID: status, Sender: "sender", Hash: "loser"},
				{ChainID: status, Sender: "sender", Hash: "next", Nonce: 1},
				{ChainID: status, Sender: "other", Hash: "other-sender"},
				{ChainID: status + "-other", Sender: "sender", Hash: "other-chain"},
			}
			for _, a := range attempts {
				a.Kind, a.SignedTx, a.SubmittedAt = TxKindRecv, []byte{1}, time.Now().UTC()
				_, err := db.InsertTxAttempt(ctx, a)
				require.NoError(t, err)
			}
			outcome := TxOutcome{ChainID: status, Sender: "sender", Hash: "winner", Status: status}
			loserStatus := TxReplaced
			if status == TxConsumed {
				outcome.Hash = ""
				loserStatus = TxConsumed
			}
			require.NoError(t, db.ResolveTxNonce(ctx, outcome))
			// Repeating resolution must preserve terminal outcomes.
			require.NoError(t, db.ResolveTxNonce(ctx, TxOutcome{ChainID: status, Sender: "sender", Status: TxConsumed}))
			for i, want := range []string{status, loserStatus, TxPending, TxPending, TxPending} {
				a, err := db.GetTxAttempt(ctx, attempts[i].ChainID, attempts[i].Hash)
				require.NoError(t, err)
				require.Equal(t, want, a.Status)
			}
		})
	}
}
