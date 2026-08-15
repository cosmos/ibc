// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/tests"
)

const (
	chainIDEth  = "1"
	chainIDBase = "8453"
	txHashEth   = "0xdeadbeef"
)

func TestStore(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		filename := filepath.Join(t.TempDir(), "ibc.db")
		t.Logf("filename: %s", filename)

		// Given a DB
		db, err := NewSqlite(filename)
		require.NoError(t, err)

		// Ensure migrations are applied
		testMigrationIdempotency(t, db)

		// Ensure foreign keys are enforced
		var foreignKeys int64
		require.NoError(t, db.db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys))
		require.EqualValues(t, 1, foreignKeys)

		// ACT + ASSERT
		testRepoReadWrite(t, db)

		// Close DB for a subsequent test
		require.NoError(t, db.Close())

		t.Run("reopen", func(t *testing.T) {
			// ARRANGE
			// Reopen the database file
			db, err := NewSqlite(filename)
			require.NoError(t, err)

			defer func() { _ = db.Close() }()

			// Ensure migrations are applied
			ensureMigrated(t, db)

			// ACT
			// Get the previously inserted relay request
			entry, err := db.GetRelayRequest(ctx, chainIDEth, txHashEth)

			// ASSERT
			require.NoError(t, err)
			assert.Equal(t, chainIDEth, entry.ChainID)
			assert.Equal(t, txHashEth, entry.TxHash)
			assert.NotZero(t, entry.ID)
		})
	})

	t.Run("sqliteInMemory", func(t *testing.T) {
		// ARRANGE
		db, err := NewSqliteInMemory()
		require.NoError(t, err)

		defer func() { _ = db.Close() }()

		testMigrationIdempotency(t, db)

		// ACT + ASSERT
		testRepoReadWrite(t, db)
	})

	t.Run("postgres", func(t *testing.T) {
		tests.GuardPostgresTests(t)

		// ARRANGE
		// Create postgres container
		ctx := context.Background()
		pg := tests.NewPostgresContainer(t)

		// Create DB
		const dbName = "ibc-link"
		pg.CreateDB(dbName)

		// Given a DB
		db, err := NewPostgres(ctx, pg.URL(dbName))
		require.NoError(t, err)

		defer func() { _ = db.Close() }()

		// Ensure migrations are applied
		testMigrationIdempotency(t, db)

		// ACT + ASSERT
		testRepoReadWrite(t, db)
	})
}

func testMigrationIdempotency(t *testing.T, m Migrator) {
	t.Helper()

	_, err := m.MigrateUp()
	require.NoError(t, err)

	_, err = m.MigrateDown()
	require.NoError(t, err)

	// Migrate up again to check if it's idempotent
	_, err = m.MigrateUp()
	require.NoError(t, err)

	ensureMigrated(t, m)
}

func ensureMigrated(t *testing.T, m Migrator) {
	t.Helper()

	// Ensure everything is migrated
	migrationStatuses, err := m.MigrationStatus()
	require.NoError(t, err)

	for _, status := range migrationStatuses {
		require.True(t, status.Applied)
	}
}

func testRepoReadWrite(t *testing.T, s Store) {
	ctx := context.Background()

	t.Run("relayRequests", func(t *testing.T) {
		// Get a non-existent relay request
		_, err := s.GetRelayRequest(ctx, chainIDEth, txHashEth)
		require.ErrorIs(t, err, ErrNotFound)

		// Insert a relay request
		err = s.CreateRelayRequest(ctx, chainIDEth, txHashEth)
		require.NoError(t, err)

		// Get the inserted relay request
		request, err := s.GetRelayRequest(ctx, chainIDEth, txHashEth)
		require.NoError(t, err)
		assert.Equal(t, chainIDEth, request.ChainID)
		assert.Equal(t, txHashEth, request.TxHash)
		assert.Equal(t, int64(1), request.ID)
		assert.NotEmpty(t, request.CreatedAt)

		// Create the same relay request again (noop)
		err = s.CreateRelayRequest(ctx, chainIDEth, txHashEth)
		require.NoError(t, err)

		requestAgain, err := s.GetRelayRequest(ctx, chainIDEth, txHashEth)
		require.NoError(t, err)
		assert.Equal(t, request.CreatedAt, requestAgain.CreatedAt)
	})

	t.Run("transact", func(t *testing.T) {
		const txHashAtomic = "0xa70m1c"

		packet := UpsertPacket{
			Status:                    RelayStatusPending,
			SourceChainID:             chainIDEth,
			DestinationChainID:        chainIDBase,
			SourceTxHash:              txHashAtomic,
			SourceTxTime:              time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC),
			PacketSequenceNumber:      7,
			PacketSourceClientID:      "base-0",
			PacketDestinationClientID: "ethereum-0",
			PacketTimeoutTimestamp:    time.Date(2026, 7, 8, 13, 0, 0, 0, time.UTC),
		}

		// A failing fn rolls back every write
		err := s.Transact(ctx, func(repo Repository) error {
			if err := repo.CreateRelayRequest(ctx, chainIDEth, txHashAtomic); err != nil {
				return err
			}
			if err := repo.UpsertPacket(ctx, packet); err != nil {
				return err
			}

			return assert.AnError
		})
		require.ErrorIs(t, err, assert.AnError)

		_, err = s.GetRelayRequest(ctx, chainIDEth, txHashAtomic)
		require.ErrorIs(t, err, ErrNotFound)

		packets, err := s.ListPacketsBySourceTx(ctx, chainIDEth, txHashAtomic)
		require.NoError(t, err)
		assert.Empty(t, packets)

		// A successful fn commits every write
		err = s.Transact(ctx, func(repo Repository) error {
			if createErr := repo.CreateRelayRequest(ctx, chainIDEth, txHashAtomic); createErr != nil {
				return createErr
			}

			return repo.UpsertPacket(ctx, packet)
		})
		require.NoError(t, err)

		_, err = s.GetRelayRequest(ctx, chainIDEth, txHashAtomic)
		require.NoError(t, err)

		packets, err = s.ListPacketsBySourceTx(ctx, chainIDEth, txHashAtomic)
		require.NoError(t, err)
		require.Len(t, packets, 1)
		assert.Equal(t, uint64(7), packets[0].PacketSequenceNumber)
	})

	t.Run("packets", func(t *testing.T) {
		packet := UpsertPacket{
			Status:                    RelayStatusPending,
			SourceChainID:             chainIDEth,
			DestinationChainID:        chainIDBase,
			SourceTxHash:              txHashEth,
			SourceTxTime:              time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC),
			PacketSequenceNumber:      42,
			PacketSourceClientID:      "base-0",
			PacketDestinationClientID: "ethereum-0",
			PacketTimeoutTimestamp:    time.Date(2026, 7, 8, 13, 0, 0, 0, time.UTC),
		}

		// No packets for the source tx yet
		packets, err := s.ListPacketsBySourceTx(ctx, chainIDEth, txHashEth)
		require.NoError(t, err)
		assert.Empty(t, packets)

		// Insert a packet
		require.NoError(t, s.UpsertPacket(ctx, packet))

		// Insert the same packet again (noop)
		require.NoError(t, s.UpsertPacket(ctx, packet))

		// Insert a second packet from the same tx
		second := packet
		second.PacketSequenceNumber = 43
		require.NoError(t, s.UpsertPacket(ctx, second))

		// List packets for the source tx
		packets, err = s.ListPacketsBySourceTx(ctx, chainIDEth, txHashEth)
		require.NoError(t, err)
		require.Len(t, packets, 2)

		// Ordered by sequence, defaults applied, round-trips intact
		first := packets[0]
		assert.Equal(t, uint64(42), first.PacketSequenceNumber)
		assert.Equal(t, uint64(43), packets[1].PacketSequenceNumber)
		assert.Equal(t, RelayStatusPending, first.Status)
		assert.Equal(t, chainIDEth, first.SourceChainID)
		assert.Equal(t, chainIDBase, first.DestinationChainID)
		assert.Equal(t, "base-0", first.PacketSourceClientID)
		assert.Equal(t, "ethereum-0", first.PacketDestinationClientID)
		assert.Equal(t, packet.SourceTxTime, first.SourceTxTime)
		assert.Equal(t, packet.PacketTimeoutTimestamp, first.PacketTimeoutTimestamp)
		assert.NotZero(t, first.CreatedAt)
		assert.NotZero(t, first.UpdatedAt)
		assert.Nil(t, first.RecvTxHash)
		assert.Nil(t, first.WriteAckStatus)

		// Invalid packet is rejected
		invalid := packet
		invalid.SourceChainID = ""
		require.ErrorContains(t, s.UpsertPacket(ctx, invalid), "source chain id is required")
		invalid = packet
		invalid.Status = RelayStatusFailed
		require.ErrorContains(t, s.UpsertPacket(ctx, invalid), "status must be NOT_SELECTED or PENDING")

		// Unknown tx returns empty list, not an error
		packets, err = s.ListPacketsBySourceTx(ctx, chainIDEth, "0xunknown")
		require.NoError(t, err)
		assert.Empty(t, packets)
	})

	t.Run("packetSelection", func(t *testing.T) {
		const (
			txHashSelection   = "0xselection"
			txHashReplacement = "0xreplacement"
		)
		input := UpsertPacket{
			Status:                    RelayStatusNotSelected,
			SourceChainID:             chainIDEth,
			DestinationChainID:        chainIDBase,
			SourceTxHash:              txHashSelection,
			SourceTxTime:              time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC),
			PacketSequenceNumber:      76,
			PacketSourceClientID:      "base-0",
			PacketDestinationClientID: "ethereum-0",
			PacketTimeoutTimestamp:    time.Date(2026, 7, 13, 13, 0, 0, 0, time.UTC),
		}
		require.NoError(t, s.UpsertPacket(ctx, input))
		key := PacketKey{SourceChainID: chainIDEth, SourceClientID: "base-0", Sequence: 76}
		fetch := func(txHash string) Packet {
			packets, err := s.ListPacketsBySourceTx(ctx, chainIDEth, txHash)
			require.NoError(t, err)
			require.Len(t, packets, 1)
			return packets[0]
		}

		dispatchable, err := s.ListDispatchablePackets(ctx)
		require.NoError(t, err)
		for _, packet := range dispatchable {
			assert.NotEqual(t, txHashSelection, packet.SourceTxHash)
		}

		input.DestinationChainID = "10"
		input.SourceTxHash = txHashReplacement
		input.SourceTxTime = input.SourceTxTime.Add(time.Hour)
		input.PacketDestinationClientID = "optimism-0"
		input.PacketTimeoutTimestamp = input.PacketTimeoutTimestamp.Add(time.Hour)
		require.NoError(t, s.UpsertPacket(ctx, input))

		packets, err := s.ListPacketsBySourceTx(ctx, chainIDEth, txHashSelection)
		require.NoError(t, err)
		assert.Empty(t, packets)
		unselected := fetch(txHashReplacement)
		assert.Equal(t, RelayStatusNotSelected, unselected.Status)
		assert.Equal(t, input.DestinationChainID, unselected.DestinationChainID)
		assert.Equal(t, input.SourceTxTime, unselected.SourceTxTime)
		assert.Equal(t, input.PacketDestinationClientID, unselected.PacketDestinationClientID)
		assert.Equal(t, input.PacketTimeoutTimestamp, unselected.PacketTimeoutTimestamp)

		input.Status = RelayStatusPending
		input.PacketTimeoutTimestamp = input.PacketTimeoutTimestamp.Add(time.Hour)
		require.NoError(t, s.UpsertPacket(ctx, input))
		selected := fetch(txHashReplacement)
		assert.Equal(t, RelayStatusPending, selected.Status)
		assert.Equal(t, input.PacketTimeoutTimestamp, selected.PacketTimeoutTimestamp)

		require.NoError(t, s.UpdatePacketStatus(ctx, key, RelayStatusDeliverRecvPacket))
		require.NoError(t, s.UpsertPacket(ctx, input))
		assert.Equal(t, RelayStatusDeliverRecvPacket, fetch(txHashReplacement).Status)
	})

	t.Run("packetLifecycle", func(t *testing.T) {
		const txHashLifecycle = "0xlifecycle"

		input := UpsertPacket{
			Status:                    RelayStatusPending,
			SourceChainID:             chainIDEth,
			DestinationChainID:        chainIDBase,
			SourceTxHash:              txHashLifecycle,
			SourceTxTime:              time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC),
			PacketSequenceNumber:      77,
			PacketSourceClientID:      "base-0",
			PacketDestinationClientID: "ethereum-0",
			PacketTimeoutTimestamp:    time.Date(2026, 7, 14, 13, 0, 0, 0, time.UTC),
		}
		require.NoError(t, s.UpsertPacket(ctx, input))

		key := PacketKey{SourceChainID: chainIDEth, SourceClientID: "base-0", Sequence: 77}
		fetch := func() Packet {
			packets, err := s.ListPacketsBySourceTx(ctx, chainIDEth, txHashLifecycle)
			require.NoError(t, err)
			require.Len(t, packets, 1)

			return packets[0]
		}
		inDispatchable := func() bool {
			dispatchable, err := s.ListDispatchablePackets(ctx)
			require.NoError(t, err)
			for _, p := range dispatchable {
				if p.SourceTxHash == txHashLifecycle {
					return true
				}
			}

			return false
		}

		require.True(t, inDispatchable())

		// recv tx set and cleared
		recvTime := time.Date(2026, 7, 14, 12, 1, 0, 0, time.UTC)
		require.NoError(t, s.UpdatePacketStatus(ctx, key, RelayStatusDeliverRecvPacket))
		require.NoError(
			t,
			s.UpdatePacketRecvTx(ctx, key, PacketTx{Hash: "0xrecv", Time: recvTime, RelayerAddress: "0xrelayer"}),
		)

		got := fetch()
		assert.Equal(t, RelayStatusDeliverRecvPacket, got.Status)
		require.NotNil(t, got.RecvTxHash)
		assert.Equal(t, "0xrecv", *got.RecvTxHash)
		assert.Equal(t, recvTime, got.RecvTxTime.UTC())
		assert.Equal(t, "0xrelayer", *got.RecvTxRelayerAddress)

		require.NoError(t, s.ClearPacketRecvTx(ctx, key))
		got = fetch()
		assert.Nil(t, got.RecvTxHash)
		assert.Nil(t, got.RecvTxTime)
		assert.Nil(t, got.RecvTxRelayerAddress)

		// write ack
		ackTime := time.Date(2026, 7, 14, 12, 2, 0, 0, time.UTC)
		require.NoError(
			t,
			s.UpdatePacketWriteAck(
				ctx,
				key,
				WriteAck{TxHash: "0xwriteack", TxTime: ackTime, Status: WriteAckStatusSuccess},
			),
		)
		got = fetch()
		assert.Equal(t, "0xwriteack", *got.WriteAckTxHash)
		assert.Equal(t, ackTime, got.WriteAckTxTime.UTC())
		assert.Equal(t, WriteAckStatusSuccess, *got.WriteAckStatus)

		// ack tx set and cleared
		require.NoError(
			t,
			s.UpdatePacketAckTx(ctx, key, PacketTx{Hash: "0xack", Time: ackTime, RelayerAddress: "0xrelayer"}),
		)
		got = fetch()
		assert.Equal(t, "0xack", *got.AckTxHash)

		require.NoError(t, s.ClearPacketAckTx(ctx, key))
		got = fetch()
		assert.Nil(t, got.AckTxHash)
		assert.Nil(t, got.AckTxTime)
		assert.Nil(t, got.AckTxRelayerAddress)

		// timeout tx set and cleared
		require.NoError(
			t,
			s.UpdatePacketTimeoutTx(ctx, key, PacketTx{Hash: "0xtimeout", Time: ackTime, RelayerAddress: "0xrelayer"}),
		)
		got = fetch()
		assert.Equal(t, "0xtimeout", *got.TimeoutTxHash)

		require.NoError(t, s.ClearPacketTimeoutTx(ctx, key))
		got = fetch()
		assert.Nil(t, got.TimeoutTxHash)

		// terminal status leaves the dispatchable set
		require.NoError(t, s.UpdatePacketStatus(ctx, key, RelayStatusCompleteWithAck))
		assert.False(t, inDispatchable())

		// updates to a different key are noops for this packet
		other := PacketKey{SourceChainID: chainIDEth, SourceClientID: "base-0", Sequence: 78}
		require.NoError(
			t,
			s.UpdatePacketRecvTx(ctx, other, PacketTx{Hash: "0xother", Time: recvTime, RelayerAddress: "0xrelayer"}),
		)
		assert.Nil(t, fetch().RecvTxHash)
	})
}
