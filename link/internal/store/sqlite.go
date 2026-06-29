package store

import (
	"context"
	"database/sql"
	"log/slog"
	"net/url"
	"path/filepath"
	"time"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/config"

	_ "modernc.org/sqlite"

	reposqlite "github.com/cosmos/ibc/link/internal/store/repository/sqlite"
	migrate "github.com/rubenv/sql-migrate"
)

// SqliteInMemory tells sqlite to use a fully in-memory database
// Useful for testing and development.
const SqliteInMemory = ":memory:"

// SqliteDB is a wrapper around the sqlite database.
type SqliteDB struct {
	db     *sql.DB
	repo   *reposqlite.Queries
	logger *slog.Logger
}

var _ Store = (*SqliteDB)(nil)

func DefaultSqliteConnOptions() map[string]string {
	return map[string]string{
		"journal_mode": "WAL", // Write-Ahead Logging mode
		"mode":         "rwc", // read, write, create file
	}
}

func NewSqlite(path string) (*SqliteDB, error) {
	return NewSqliteWithOptions(path, DefaultSqliteConnOptions())
}

func NewSqliteInMemory() (*SqliteDB, error) {
	return NewSqliteWithOptions(SqliteInMemory, nil)
}

func NewSqliteWithOptions(path string, connectionOpts map[string]string) (*SqliteDB, error) {
	if path == "" {
		return nil, errors.New("path is required")
	}

	logger := slog.With("module", "database")

	if path == SqliteInMemory {
		db, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, errors.Wrapf(err, "open sqlite in memory")
		}

		// shared memory database is a single connection
		db.SetMaxOpenConns(1)

		return &SqliteDB{
			db:     db,
			repo:   reposqlite.New(db),
			logger: logger,
		}, nil
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, errors.Wrapf(err, "absolute path for %s", path)
	}

	path = absPath

	connectionString, err := sqliteURL(path, connectionOpts)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", connectionString)
	if err != nil {
		return nil, errors.Wrapf(err, "open sqlite database")
	}

	return &SqliteDB{
		db:     db,
		repo:   reposqlite.New(db),
		logger: logger,
	}, nil
}

func (db *SqliteDB) Close() error {
	return db.db.Close()
}

// MigrateUp migrates ALL available migrations
func (db *SqliteDB) MigrateUp() (int, error) {
	return migrateDB(db.db, config.DBTypeSQLite, migrate.Up, 0)
}

// MigrateDown migrates only ONE migration down
func (db *SqliteDB) MigrateDown() (int, error) {
	return migrateDB(db.db, config.DBTypeSQLite, migrate.Down, 1)
}

func (db *SqliteDB) MigrationStatus() ([]MigrationStatus, error) {
	return migrationStatus(db.db, config.DBTypeSQLite)
}

func (db *SqliteDB) GetRelaySubmission(ctx context.Context, chainID string, txHash string) (*RelaySubmission, error) {
	if chainID == "" || txHash == "" {
		return nil, errors.New("chainID and txHash are required")
	}

	entry, err := db.repo.GetRelaySubmission(ctx, chainID, txHash)
	if err != nil {
		return nil, errNormalize(err)
	}

	// todo: time as timestamp

	return &RelaySubmission{
		ID:        entry.ID,
		ChainID:   entry.SourceChainID,
		TxHash:    entry.SourceTxHash,
		CreatedAt: time.Now().UTC(), // todo
	}, nil
}

func (db *SqliteDB) UpsertRelaySubmission(ctx context.Context, chainID string, txHash string) error {
	if chainID == "" || txHash == "" {
		return errors.New("chainID and txHash are required")
	}

	return db.repo.UpsertRelaySubmission(ctx, chainID, txHash)
}

func sqliteURL(path string, connectionOpts map[string]string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", errors.Wrapf(err, "absolute path for %s", path)
	}

	path = absPath

	u := url.URL{
		Scheme: "file",
		Path:   path,
	}

	query := u.Query()
	for k, v := range connectionOpts {
		query.Set(k, v)
	}

	u.RawQuery = query.Encode()

	return u.String(), nil
}
