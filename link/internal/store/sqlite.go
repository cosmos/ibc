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
	db     *sql.DB
	repo   *reposqlite.Queries
	logger *slog.Logger
}

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

	if path == SqliteInMemory {
		db, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, errors.Wrapf(err, "open sqlite in memory")
		}

		return &SqliteDB{db: db}, nil
	}

	if path != SqliteInMemory {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return nil, errors.Wrapf(err, "absolute path for %s", path)
		}

		path = absPath
	}

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
		logger: slog.With("module", "database"),
	}, nil
}

func (db *SqliteDB) Close() error {
	return db.db.Close()
}

func (db *SqliteDB) MigrateUp() (int, error) {
	return migrateDB(db.db, config.DBTypeSQLite, migrate.Up)
}

func (db *SqliteDB) MigrateDown() (int, error) {
	return migrateDB(db.db, config.DBTypeSQLite, migrate.Down)
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
