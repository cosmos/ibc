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

			defer db.Close()

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

		defer db.Close()

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

		defer db.Close()

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

	t.Run("execTx", func(t *testing.T) {
		const txHashAtomic = "0xa70m1c"

		packet := CreatePacket{
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
		err := s.ExecTx(ctx, func(repo Repository) error {
			if err := repo.CreateRelayRequest(ctx, chainIDEth, txHashAtomic); err != nil {
				return err
			}
			if err := repo.CreatePacket(ctx, packet); err != nil {
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
		err = s.ExecTx(ctx, func(repo Repository) error {
			if err := repo.CreateRelayRequest(ctx, chainIDEth, txHashAtomic); err != nil {
				return err
			}

			return repo.CreatePacket(ctx, packet)
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
		packet := CreatePacket{
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
		require.NoError(t, s.CreatePacket(ctx, packet))

		// Insert the same packet again (noop)
		require.NoError(t, s.CreatePacket(ctx, packet))

		// Insert a second packet from the same tx
		second := packet
		second.PacketSequenceNumber = 43
		require.NoError(t, s.CreatePacket(ctx, second))

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
		assert.Nil(t, first.StatusText)

		// Invalid packet is rejected
		invalid := packet
		invalid.SourceChainID = ""
		require.ErrorContains(t, s.CreatePacket(ctx, invalid), "source chain id is required")

		// Unknown tx returns empty list, not an error
		packets, err = s.ListPacketsBySourceTx(ctx, chainIDEth, "0xunknown")
		require.NoError(t, err)
		assert.Empty(t, packets)
	})
}
