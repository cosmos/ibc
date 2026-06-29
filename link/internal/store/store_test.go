package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/cosmos/ibc/link/internal/store/repository/postgres"
	"github.com/cosmos/ibc/link/internal/store/repository/sqlite"
	"github.com/cosmos/ibc/link/internal/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		t.Run("regularFile", func(t *testing.T) {
			// ARRANGE
			ctx := context.Background()
			filename := filepath.Join(t.TempDir(), "ibc.db")
			t.Logf("filename: %s", filename)

			// Given a DB
			db, err := NewSqlite(filename)
			require.NoError(t, err)

			// Ensure migrations are applied
			testMigrationIdempotency(t, db)

			// ACT
			// Insert a relay submission
			insertParams := sqlite.InsertRelaySubmissionParams{
				ChainID: "cosmoshub-4",
				TxHash:  "ABC123DEF456",
			}
			err = db.repo.InsertRelaySubmission(ctx, insertParams)

			// ASSERT
			require.NoError(t, err)

			// ACT
			// Get the inserted submission
			req := sqlite.GetRelaySubmissionParams{
				ChainID: insertParams.ChainID,
				TxHash:  insertParams.TxHash,
			}
			submission, err := db.repo.GetRelaySubmission(ctx, req)

			// ASSERT
			require.NoError(t, err)
			assert.Equal(t, insertParams.ChainID, submission.SourceChainID)
			assert.Equal(t, insertParams.TxHash, submission.SourceTxHash)
			assert.NotZero(t, submission.ID)
			assert.NotEmpty(t, submission.CreatedAt)

			// ACT
			// Upsert the submission
			err = db.repo.InsertRelaySubmission(context.Background(), insertParams)

			// ASSERT
			require.NoError(t, err)

			require.NoError(t, db.Close())

			t.Run("reopen", func(t *testing.T) {
				// ARRANGE - Reopen the database file
				db, err := NewSqlite(filename)
				require.NoError(t, err)

				defer db.Close()

				// Ensure migrations are applied
				ensureMigrated(t, db)

				// ACT
				// Get the previously inserted submission
				getParams := sqlite.GetRelaySubmissionParams{
					ChainID: "cosmoshub-4",
					TxHash:  "ABC123DEF456",
				}
				submission, err := db.repo.GetRelaySubmission(context.Background(), getParams)

				// ASSERT
				// Data should persist after reopening
				require.NoError(t, err)
				assert.Equal(t, "cosmoshub-4", submission.SourceChainID)
				assert.Equal(t, "ABC123DEF456", submission.SourceTxHash)
				assert.NotZero(t, submission.ID)
			})
		})

		t.Run("inMemory", func(t *testing.T) {
			// ARRANGE
			ctx := context.Background()
			db, err := NewSqliteInMemory()
			require.NoError(t, err)

			defer db.Close()

			testMigrationIdempotency(t, db)

			// ACT - Insert a relay submission
			insertParams := sqlite.InsertRelaySubmissionParams{
				ChainID: "1",
				TxHash:  "0xDEADBEEF",
			}
			err = db.repo.InsertRelaySubmission(ctx, insertParams)

			// ASSERT
			require.NoError(t, err)

			// ACT - Get the inserted submission
			req := sqlite.GetRelaySubmissionParams{
				ChainID: insertParams.ChainID,
				TxHash:  insertParams.TxHash,
			}
			submission, err := db.repo.GetRelaySubmission(ctx, req)

			// ASSERT
			require.NoError(t, err)
			assert.Equal(t, insertParams.ChainID, submission.SourceChainID)
			assert.Equal(t, insertParams.TxHash, submission.SourceTxHash)
			assert.NotZero(t, submission.ID)
			assert.NotEmpty(t, submission.CreatedAt)

			// ACT - Insert same submission again (upsert behavior)
			err = db.repo.InsertRelaySubmission(ctx, insertParams)

			// ASSERT
			require.NoError(t, err)
		})
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

		// ACT
		// Insert a relay submission
		insertParams := postgres.InsertRelaySubmissionParams{
			ChainID: "cosmoshub-4",
			TxHash:  "ABC123DEF456",
		}
		err = db.repo.InsertRelaySubmission(ctx, insertParams)

		// ASSERT
		require.NoError(t, err)

		// ACT
		// Get the inserted submission
		req := postgres.GetRelaySubmissionParams{
			ChainID: insertParams.ChainID,
			TxHash:  insertParams.TxHash,
		}
		submission, err := db.repo.GetRelaySubmission(ctx, req)

		// ASSERT
		require.NoError(t, err)
		assert.Equal(t, insertParams.ChainID, submission.SourceChainID)
		assert.Equal(t, insertParams.TxHash, submission.SourceTxHash)
		assert.NotZero(t, submission.ID)
		assert.NotEmpty(t, submission.CreatedAt)

		// ACT
		// Upsert the submission
		err = db.repo.InsertRelaySubmission(ctx, insertParams)

		// ASSERT
		require.NoError(t, err)
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
