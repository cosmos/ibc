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
			ChainID: "target", Sender: "sender", Hash: "first", Nonce: 7,
			SignedTx: []byte{1, 2, 3}, SubmittedAt: time.Now().UTC(),
		}
		missing := key
		missing.Sequence = 999
		_, err := SaveTxAttempt(
			ctx,
			db,
			a,
			[]TxOperation{{PacketKey: key, Type: OperationTypeRecv}, {PacketKey: missing, Type: OperationTypeRecv}},
		)
		require.Error(t, err)
		_, err = db.GetTxAttempt(ctx, a.ChainID, a.Hash)
		require.ErrorIs(t, err, ErrNotFound)
		pending, err := db.HasPendingOperation(ctx, TxOperation{PacketKey: key, Type: OperationTypeRecv})
		require.NoError(t, err)
		require.False(t, pending, "packet associations must roll back with the attempt")

		_, err = SaveTxAttempt(ctx, db, a, []TxOperation{
			{PacketKey: key, Type: OperationTypeRecv},
			{PacketKey: key, Type: "unknown"},
		})
		require.ErrorContains(t, err, "invalid operation type")
		_, err = db.GetTxAttempt(ctx, a.ChainID, a.Hash)
		require.ErrorIs(t, err, ErrNotFound)

		id, err := SaveTxAttempt(ctx, db, a, []TxOperation{{PacketKey: key, Type: OperationTypeRecv}})
		require.NoError(t, err)
		links, err := db.ListTxOperations(ctx, id)
		require.NoError(t, err)
		require.Equal(t, []TxOperation{{PacketKey: key, Type: OperationTypeRecv}}, links)
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
		_, err = SaveTxAttempt(ctx, db, replacement, []TxOperation{{PacketKey: key, Type: OperationTypeRecv}})
		require.NoError(t, err)
		next := a
		next.Hash, next.Nonce = "third", 8
		_, err = SaveTxAttempt(ctx, db, next, []TxOperation{{PacketKey: key, Type: OperationTypeRecv}})
		require.NoError(t, err)
		nonces, err := db.ListTxNoncesFrom(ctx, a.ChainID, a.Sender, 7)
		require.NoError(t, err)
		require.Equal(t, []uint64{7, 8}, nonces)
		pending, err = db.HasPendingOperation(ctx, TxOperation{PacketKey: key, Type: OperationTypeRecv})
		require.NoError(t, err)
		require.True(t, pending)
		pending, err = db.HasPendingOperation(ctx, TxOperation{PacketKey: key, Type: OperationTypeAck})
		require.NoError(t, err)
		require.False(t, pending)

		require.NoError(
			t,
			db.ResolveTxNonce(
				ctx,
				TxOutcome{
					BlockNumber: 10,
					BlockHash:   "block",
					ChainID:     a.ChainID,
					Sender:      a.Sender,
					Nonce:       a.Nonce,
					Hash:        a.Hash,
					Status:      TxConfirmed,
				},
			),
		)
		for hash, status := range map[string]string{a.Hash: TxConfirmed, replacement.Hash: TxReplaced} {
			resolved, errRead := db.GetTxAttempt(ctx, a.ChainID, hash)
			require.NoError(t, errRead)
			require.Equal(t, status, resolved.Status)
			require.NotEmpty(t, resolved.SignedTx, "history remains available")
		}
		pending, err = db.HasPendingOperation(ctx, TxOperation{PacketKey: key, Type: OperationTypeRecv})
		require.NoError(t, err)
		require.True(t, pending, "another nonce still owns this operation")
		require.NoError(
			t,
			db.ResolveTxNonce(
				ctx,
				TxOutcome{
					BlockNumber: 10,
					BlockHash:   "block",
					ChainID:     next.ChainID,
					Sender:      next.Sender,
					Nonce:       next.Nonce,
					Status:      TxConsumed,
				},
			),
		)
		pending, err = db.HasPendingOperation(ctx, TxOperation{PacketKey: key, Type: OperationTypeRecv})
		require.NoError(t, err)
		require.False(t, pending)
		attempts, err := db.ListUnresolvedTxAttempts(ctx, a.ChainID, a.Sender, 0)
		require.NoError(t, err)
		require.Empty(t, attempts)
		packets, err := db.ListPacketsBySourceTx(ctx, key.SourceChainID, "send")
		require.NoError(t, err)
		require.Nil(t, packets[0].RecvTxHash, "resolving a nonce does not establish packet completion")

		nonces, err = db.ListTxNoncesFrom(ctx, a.ChainID, a.Sender, 7)
		require.NoError(t, err)
		require.Equal(t, []uint64{7, 8}, nonces, "resolved nonces remain owned if mining rolls back")
		nonces, err = db.ListTxNoncesFrom(ctx, a.ChainID, a.Sender, 8)
		require.NoError(t, err)
		require.Equal(t, []uint64{8}, nonces)
		resolutions, err := db.ListTxResolutionsFrom(ctx, a.ChainID, a.Sender, 7)
		require.NoError(t, err)
		require.Equal(t, []TxResolution{
			{Nonce: 7, BlockNumber: 10, BlockHash: "block"},
			{Nonce: 8, BlockNumber: 10, BlockHash: "block"},
		}, resolutions, "one resolution per nonce, including consumed outcomes")
		for _, stale := range []TxResolution{
			{Nonce: 7, BlockNumber: 10, BlockHash: "different block"},
			{Nonce: 7, BlockNumber: 11, BlockHash: "block"},
		} {
			require.NoError(t, db.ReopenTxAttempts(ctx, a.ChainID, a.Sender, stale))
			attempts, err = db.ListUnresolvedTxAttempts(ctx, a.ChainID, a.Sender, 0)
			require.NoError(t, err)
			require.Empty(t, attempts, "stale observations must not overwrite another resolution")
		}
		require.NoError(
			t,
			db.ReopenTxAttempts(ctx, a.ChainID, a.Sender, TxResolution{Nonce: 7, BlockNumber: 10, BlockHash: "block"}),
		)
		pending, err = db.HasPendingOperation(ctx, TxOperation{PacketKey: key, Type: OperationTypeRecv})
		require.NoError(t, err)
		require.True(t, pending, "reopened attempts retain their packet associations")
		found, err = db.GetTxAttempt(ctx, a.ChainID, a.Hash)
		require.NoError(t, err)
		require.Nil(t, found.LastBroadcastAt, "reopened attempts are eligible for immediate rebroadcast")
		require.Equal(t, a.SignedTx, found.SignedTx)
		require.WithinDuration(t, a.SubmittedAt, found.SubmittedAt, time.Microsecond)
		require.NoError(t, db.SetTxBroadcast(ctx, a.ChainID, a.Hash, broadcast))
		require.NoError(
			t,
			db.ReopenTxAttempts(ctx, a.ChainID, a.Sender, TxResolution{Nonce: 7, BlockNumber: 10, BlockHash: "block"}),
		)
		found, err = db.GetTxAttempt(ctx, a.ChainID, a.Hash)
		require.NoError(t, err)
		require.NotNil(t, found.LastBroadcastAt, "repeated recovery preserves pending broadcast timing")
		require.WithinDuration(t, broadcast, *found.LastBroadcastAt, time.Microsecond)
		resolutions, err = db.ListTxResolutionsFrom(ctx, a.ChainID, a.Sender, 7)
		require.NoError(t, err)
		require.Equal(t, []TxResolution{{Nonce: 8, BlockNumber: 10, BlockHash: "block"}}, resolutions)
	})
	t.Run("mixedOperations", func(t *testing.T) {
		ctx := t.Context()
		key := PacketKey{SourceChainID: "mixed", SourceClientID: "source", Sequence: 1}
		require.NoError(t, db.UpsertPacket(ctx, UpsertPacket{
			Status: RelayStatusPending, SourceChainID: key.SourceChainID, DestinationChainID: "target",
			SourceTxHash: "send", SourceTxTime: time.Now().UTC(), PacketSequenceNumber: key.Sequence,
			PacketSourceClientID: key.SourceClientID, PacketDestinationClientID: "dest",
			PacketTimeoutTimestamp: time.Now().Add(time.Hour),
		}))
		operations := []TxOperation{
			{PacketKey: key, Type: OperationTypeRecv},
			{PacketKey: key, Type: OperationTypeAck},
		}
		attempt := TxAttempt{
			ChainID: "target", Sender: "sender", Hash: "mixed",
			SignedTx: []byte{1}, SubmittedAt: time.Now().UTC(),
		}
		id, err := SaveTxAttempt(ctx, db, attempt, operations)
		require.NoError(t, err)
		stored, err := db.ListTxOperations(ctx, id)
		require.NoError(t, err)
		require.ElementsMatch(t, operations, stored)
		for _, operation := range operations {
			pending, lookupErr := db.HasPendingOperation(ctx, operation)
			require.NoError(t, lookupErr)
			require.True(t, pending)
		}
		pending, err := db.HasPendingOperation(ctx, TxOperation{PacketKey: key, Type: OperationTypeTimeout})
		require.NoError(t, err)
		require.False(t, pending)
		require.NoError(t, db.ResolveTxNonce(ctx, TxOutcome{
			ChainID: attempt.ChainID, Sender: attempt.Sender, Nonce: attempt.Nonce,
			Hash: attempt.Hash, Status: TxConfirmed, BlockNumber: 1, BlockHash: "block",
		}))
		for _, operation := range operations {
			pending, lookupErr := db.HasPendingOperation(ctx, operation)
			require.NoError(t, lookupErr)
			require.False(t, pending)
		}
	})
	t.Run("abandonment", func(t *testing.T) {
		ctx := t.Context()
		key := PacketKey{SourceChainID: "abandonment", SourceClientID: "source", Sequence: 1}
		require.NoError(t, db.UpsertPacket(ctx, UpsertPacket{
			Status: RelayStatusPending, SourceChainID: key.SourceChainID, DestinationChainID: "target",
			SourceTxHash: "send", SourceTxTime: time.Now().UTC(), PacketSequenceNumber: key.Sequence,
			PacketSourceClientID: key.SourceClientID, PacketDestinationClientID: "dest",
			PacketTimeoutTimestamp: time.Now().Add(time.Hour),
		}))
		operation := TxOperation{PacketKey: key, Type: OperationTypeRecv}
		first := TxAttempt{
			ChainID: "abandonment", Sender: "sender", Hash: "first", Nonce: 1,
			SignedTx: []byte{1}, SubmittedAt: time.Now().UTC().Add(-time.Hour),
		}
		id, err := SaveTxAttempt(ctx, db, first, []TxOperation{operation})
		require.NoError(t, err)
		second := first
		second.Hash = "replacement"
		_, err = SaveTxAttempt(ctx, db, second, []TxOperation{operation})
		require.NoError(t, err)
		next := first
		next.Hash, next.Nonce = "next", 2
		_, err = SaveTxAttempt(ctx, db, next, nil)
		require.NoError(t, err)

		require.NoError(t, db.AbandonTxAttempts(ctx, first.ChainID, first.Sender, first.Nonce))
		pending, err := db.HasPendingOperation(ctx, operation)
		require.NoError(t, err)
		require.False(t, pending)
		nonces, err := db.ListTxNoncesFrom(ctx, first.ChainID, first.Sender, 0)
		require.NoError(t, err)
		require.Equal(t, []uint64{2}, nonces)
		unresolved, err := db.ListUnresolvedTxAttempts(ctx, first.ChainID, first.Sender, 1)
		require.NoError(t, err)
		require.Len(t, unresolved, 1)
		require.Equal(t, next.Hash, unresolved[0].Hash)
		unresolved, err = db.ListUnresolvedTxAttempts(ctx, first.ChainID, first.Sender, 2)
		require.NoError(t, err)
		require.Len(t, unresolved, 3, "consumed abandoned nonces still need receipt reconciliation")

		first.SubmittedAt = time.Now().UTC()
		reusedID, err := SaveTxAttempt(ctx, db, first, []TxOperation{operation})
		require.NoError(t, err)
		require.Equal(t, id, reusedID)
		operations, err := db.ListTxOperations(ctx, id)
		require.NoError(t, err)
		require.Equal(t, []TxOperation{operation}, operations)
		pending, err = db.HasPendingOperation(ctx, operation)
		require.NoError(t, err)
		require.True(t, pending)
		require.NoError(t, db.ResolveTxNonce(ctx, TxOutcome{
			ChainID: first.ChainID, Sender: first.Sender, Nonce: first.Nonce,
			Hash: second.Hash, Status: TxConfirmed, BlockNumber: 10, BlockHash: "block",
		}))
		for hash, status := range map[string]string{first.Hash: TxReplaced, second.Hash: TxConfirmed, next.Hash: TxPending} {
			attempt, readErr := db.GetTxAttempt(ctx, first.ChainID, hash)
			require.NoError(t, readErr)
			require.Equal(t, status, attempt.Status)
		}
	})
	for _, status := range []string{TxConfirmed, TxFailed, TxConsumed} {
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
				a.SignedTx, a.SubmittedAt = []byte{1}, time.Now().UTC()
				_, err := db.InsertTxAttempt(ctx, a)
				require.NoError(t, err)
			}
			outcome := TxOutcome{
				BlockNumber: 10,
				BlockHash:   "block",
				ChainID:     status,
				Sender:      "sender",
				Hash:        "winner",
				Status:      status,
			}
			loserStatus := TxReplaced
			if status == TxConsumed {
				outcome.Hash = ""
				loserStatus = TxConsumed
			}
			require.NoError(t, db.ResolveTxNonce(ctx, outcome))
			// Repeating resolution must preserve terminal outcomes.
			require.NoError(
				t,
				db.ResolveTxNonce(
					ctx,
					TxOutcome{
						BlockNumber: 10,
						BlockHash:   "block",
						ChainID:     status,
						Sender:      "sender",
						Status:      TxConsumed,
					},
				),
			)
			for i, want := range []string{status, loserStatus, TxPending, TxPending, TxPending} {
				a, err := db.GetTxAttempt(ctx, attempts[i].ChainID, attempts[i].Hash)
				require.NoError(t, err)
				require.Equal(t, want, a.Status)
			}

			for _, a := range attempts[2:] {
				require.NoError(t, db.ResolveTxNonce(ctx, TxOutcome{
					BlockNumber: 10, BlockHash: "block",
					ChainID: a.ChainID, Sender: a.Sender, Nonce: a.Nonce, Status: TxConsumed,
				}))
			}
			require.NoError(
				t,
				db.ReopenTxAttempts(ctx, status, "sender", TxResolution{Nonce: 1, BlockNumber: 10, BlockHash: "block"}),
			)
			for i, want := range []string{status, loserStatus, TxPending, TxConsumed, TxConsumed} {
				a, err := db.GetTxAttempt(ctx, attempts[i].ChainID, attempts[i].Hash)
				require.NoError(t, err)
				require.Equal(t, want, a.Status, "only unmined nonces for this account reopen")
			}
			require.NoError(
				t,
				db.ReopenTxAttempts(ctx, status, "sender", TxResolution{Nonce: 0, BlockNumber: 10, BlockHash: "block"}),
			)
			for i, want := range []string{TxPending, TxPending, TxPending, TxConsumed, TxConsumed} {
				a, err := db.GetTxAttempt(ctx, attempts[i].ChainID, attempts[i].Hash)
				require.NoError(t, err)
				require.Equal(t, want, a.Status)
			}
		})
	}
}
