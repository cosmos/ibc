package store

import (
	"context"
	"path"
	"path/filepath"
	"sort"
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

var ethSubmission = RelaySubmissionKey{ChainID: chainIDEth, TxHash: txHashEth}

func TestStore(t *testing.T) {

	t.Run("sqlite", func(t *testing.T) {
		ctx := context.Background()
		filename := filepath.Join(t.TempDir(), "ibc.db")
		t.Logf("filename: %s", filename)

		db, err := newSQLite(filename)
		require.NoError(t, err)

		testMigrationIdempotency(t, db)
		testSQLiteWAL(t, db)

		testRepoReadWrite(t, db)

		require.NoError(t, db.Close())

		t.Run("reopen", func(t *testing.T) {
			db, err := newSQLite(filename)
			require.NoError(t, err)

			defer db.Close()

			ensureMigrated(t, db)

			entry, err := db.GetRelaySubmission(ctx, ethSubmission)

			require.NoError(t, err)
			assert.Equal(t, chainIDEth, entry.ChainID)
			assert.Equal(t, txHashEth, entry.TxHash)
			assert.NotZero(t, entry.ID)
		})
	})

	t.Run("sqliteInMemory", func(t *testing.T) {
		db, err := newSQLiteWithOptions(sqliteInMemory, nil)
		require.NoError(t, err)

		defer db.Close()

		testMigrationIdempotency(t, db)

		testRepoReadWrite(t, db)
	})

	t.Run("postgres", func(t *testing.T) {
		tests.GuardPostgresTests(t)

		ctx := context.Background()
		pg := tests.NewPostgresContainer(t)

		const dbName = "ibc-link"
		pg.CreateDB(dbName)

		db, err := newPostgres(ctx, pg.URL(dbName))
		require.NoError(t, err)

		defer db.Close()

		testMigrationIdempotency(t, db)

		testRepoReadWrite(t, db)
	})
}

func TestNewDatabaseCreatesUsableStore(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DB.URL = filepath.Join(t.TempDir(), "ibc.db")

	db, err := NewDatabase(context.Background(), cfg)
	require.NoError(t, err)

	testMigrationIdempotency(t, db)

	require.NoError(t, db.Close())
}

func TestMigrationFilesMatchDialects(t *testing.T) {
	sqliteMigrations := migrationFiles(t, config.DBTypeSQLite)
	postgresMigrations := migrationFiles(t, config.DBTypePostgres)

	assert.Equal(t, sqliteMigrations, postgresMigrations)
}

func testMigrationIdempotency(t *testing.T, m Migrator) {
	t.Helper()
	ctx := context.Background()

	_, err := m.MigrateUp(ctx)
	require.NoError(t, err)

	_, err = m.MigrateDown(ctx)
	require.NoError(t, err)

	_, err = m.MigrateUp(ctx)
	require.NoError(t, err)

	ensureMigrated(t, m)
}

func migrationFiles(t *testing.T, dbType string) []string {
	t.Helper()

	entries, err := migrationsFS.ReadDir(path.Join("migrations", dbType))
	require.NoError(t, err)

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".sql" {
			files = append(files, entry.Name())
		}
	}

	sort.Strings(files)
	return files
}

func ensureMigrated(t *testing.T, m Migrator) {
	t.Helper()

	migrationStatuses, err := m.MigrationStatus()
	require.NoError(t, err)

	for _, status := range migrationStatuses {
		require.True(t, status.Applied)
	}
}

func testRepoReadWrite(t *testing.T, s Store) {
	ctx := context.Background()

	_, err := s.GetRelaySubmission(ctx, RelaySubmissionKey{TxHash: txHashEth})
	require.ErrorIs(t, err, ErrMissingChainTx)

	err = s.UpsertRelaySubmission(ctx, RelaySubmissionKey{ChainID: chainIDEth})
	require.ErrorIs(t, err, ErrMissingChainTx)

	_, err = s.GetRelaySubmission(ctx, ethSubmission)
	require.ErrorIs(t, err, ErrNotFound)

	err = s.UpsertRelaySubmission(ctx, ethSubmission)
	require.NoError(t, err)

	submission, err := s.GetRelaySubmission(ctx, ethSubmission)
	require.NoError(t, err)
	assert.Equal(t, chainIDEth, submission.ChainID)
	assert.Equal(t, txHashEth, submission.TxHash)
	assert.NotZero(t, submission.ID)
	assert.NotEmpty(t, submission.CreatedAt)

	submissionAgain, err := s.GetRelaySubmission(ctx, ethSubmission)
	require.NoError(t, err)
	assert.Equal(t, submission.CreatedAt, submissionAgain.CreatedAt)

	err = s.UpsertRelaySubmission(ctx, ethSubmission)
	require.NoError(t, err)
}

func testSQLiteWAL(t *testing.T, db *sqliteStore) {
	t.Helper()

	var journalMode string
	require.NoError(t, db.db.QueryRowContext(context.Background(), "PRAGMA journal_mode").Scan(&journalMode))
	assert.Equal(t, "wal", journalMode)
}
