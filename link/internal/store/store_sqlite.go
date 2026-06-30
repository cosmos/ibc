package store

import (
	"database/sql"
	"log/slog"
	"net/url"
	"path/filepath"

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
	db *sql.DB

	*repositoryStore

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

		return newSqliteDB(db, logger), nil
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

	return newSqliteDB(db, logger), nil
}

func newSqliteDB(db *sql.DB, logger *slog.Logger) *SqliteDB {
	repo := sqliteRepository{queries: reposqlite.New(db)}

	return &SqliteDB{
		db:              db,
		repositoryStore: newRepositoryStore(repo, logger),
		logger:          logger,
	}
}

func (db *SqliteDB) Close() error {
	return db.db.Close()
}

// MigrateUp migrates ALL available migrations
func (db *SqliteDB) MigrateUp() (int, error) {
	db.logger.Debug("Migrating up")
	return migrateDB(db.db, config.DBTypeSQLite, migrate.Up, 0)
}

// MigrateDown migrates only ONE migration down
func (db *SqliteDB) MigrateDown() (int, error) {
	db.logger.Debug("Migrating down")
	return migrateDB(db.db, config.DBTypeSQLite, migrate.Down, 1)
}

func (db *SqliteDB) MigrationStatus() ([]MigrationStatus, error) {
	return migrationStatus(db.db, config.DBTypeSQLite)
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
