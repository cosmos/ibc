package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	chainIDEth = "1"
	txHashEth  = "0xDEADBEEF"
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

		// ACT + ASSERT
		testRepoReadWrite(t, db)
		testTransactions(t, db)

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
			// Get the previously inserted submission
			entry, err := db.GetRelaySubmission(ctx, chainIDEth, txHashEth)

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
		testTransactions(t, db)
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
		testTransactions(t, db)
	})
}

func TestNewStoreCreatesUsableStore(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DB.URL = filepath.Join(t.TempDir(), "ibc.db")

	db, err := NewStore(context.Background(), cfg)
	require.NoError(t, err)

	testMigrationIdempotency(t, db)

	require.NoError(t, db.Close())
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

	_, err := s.GetRelaySubmission(ctx, "", txHashEth)
	require.ErrorIs(t, err, ErrMissingChainTx)

	err = s.UpsertRelaySubmission(ctx, chainIDEth, "")
	require.ErrorIs(t, err, ErrMissingChainTx)

	// Get a non-existent submission
	_, err = s.GetRelaySubmission(ctx, chainIDEth, txHashEth)
	require.ErrorIs(t, err, ErrNotFound)

	// Insert a submission
	err = s.UpsertRelaySubmission(ctx, chainIDEth, txHashEth)
	require.NoError(t, err)

	// Get the inserted submission
	submission, err := s.GetRelaySubmission(ctx, chainIDEth, txHashEth)
	require.NoError(t, err)
	assert.Equal(t, chainIDEth, submission.ChainID)
	assert.Equal(t, txHashEth, submission.TxHash)
	assert.Equal(t, int64(1), submission.ID)
	assert.NotEmpty(t, submission.CreatedAt)

	submissionAgain, err := s.GetRelaySubmission(ctx, chainIDEth, txHashEth)
	require.NoError(t, err)
	assert.Equal(t, submission.CreatedAt, submissionAgain.CreatedAt)

	// Upsert the submission (noop)
	err = s.UpsertRelaySubmission(ctx, chainIDEth, txHashEth)
	require.NoError(t, err)
}

func testTransactions(t *testing.T, s Store) {
	t.Helper()

	ctx := context.Background()
	txStore, ok := s.(transactionStore)
	require.True(t, ok)

	t.Run("transactionCommit", func(t *testing.T) {
		err := txStore.withTx(ctx, func(repo Repository) error {
			return repo.UpsertRelaySubmission(ctx, "tx-commit", "0xCOMMIT")
		})
		require.NoError(t, err)

		committed, err := s.GetRelaySubmission(ctx, "tx-commit", "0xCOMMIT")
		require.NoError(t, err)
		assert.Equal(t, "tx-commit", committed.ChainID)
		assert.Equal(t, "0xCOMMIT", committed.TxHash)
	})

	t.Run("transactionRollback", func(t *testing.T) {
		rollbackErr := errors.New("force rollback")
		err := txStore.withTx(ctx, func(repo Repository) error {
			err := repo.UpsertRelaySubmission(ctx, "tx-rollback", "0xROLLBACK")
			require.NoError(t, err)

			rolledBack, err := repo.GetRelaySubmission(ctx, "tx-rollback", "0xROLLBACK")
			require.NoError(t, err)
			assert.Equal(t, "tx-rollback", rolledBack.ChainID)

			return rollbackErr
		})
		require.ErrorIs(t, err, rollbackErr)

		_, err = s.GetRelaySubmission(ctx, "tx-rollback", "0xROLLBACK")
		require.ErrorIs(t, err, ErrNotFound)
	})
}
