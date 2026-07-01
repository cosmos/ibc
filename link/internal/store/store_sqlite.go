package store

import (
	"context"
	"database/sql"
	"log/slog"
	"net/url"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/config"

	//nolint:blank-imports // SQL driver
	_ "modernc.org/sqlite"

	reposqlite "github.com/cosmos/ibc/link/internal/store/repository/sqlite"
	migrate "github.com/rubenv/sql-migrate"
)

// SqliteInMemory tells sqlite to use a fully in-memory database
// Useful for testing and development.
const SqliteInMemory = ":memory:"

type sqliteStore struct {
	db *sql.DB

	*repositoryStore

	logger *slog.Logger
}

var (
	_ Store            = (*sqliteStore)(nil)
	_ transactionStore = (*sqliteStore)(nil)
)

func DefaultSqliteConnOptions() map[string]string {
	return map[string]string{
		"journal_mode": "WAL", // Write-Ahead Logging mode
		"mode":         "rwc", // read, write, create file
	}
}

func NewSqlite(path string) (Store, error) {
	return NewSqliteWithOptions(path, DefaultSqliteConnOptions())
}

func NewSqliteInMemory() (Store, error) {
	return NewSqliteWithOptions(SqliteInMemory, nil)
}

func NewSqliteWithOptions(path string, connectionOpts map[string]string) (Store, error) {
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

		return newSqliteStore(db, logger), nil
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

	// sqlite can auto-create db files, but not nested directories.
	if err = config.EnsureDirectory(path); err != nil {
		return nil, errors.Wrapf(err, "ensure directory for %s", path)
	}

	db, err := sql.Open("sqlite", connectionString)
	if err != nil {
		return nil, errors.Wrapf(err, "open sqlite database %s", connectionString)
	}

	return newSqliteStore(db, logger), nil
}

func newSqliteStore(db *sql.DB, logger *slog.Logger) Store {
	repo := sqliteRepository{queries: reposqlite.New(db)}

	return &sqliteStore{
		db:              db,
		repositoryStore: newRepositoryStore(repo, logger),
		logger:          logger,
	}
}

func (d *sqliteStore) Close() error {
	return d.db.Close()
}

func (d *sqliteStore) Ping(_ context.Context) error {
	return d.db.Ping()
}

// MigrateUp migrates ALL available migrations
func (d *sqliteStore) MigrateUp() (int, error) {
	d.logger.Debug("Migrating up")
	return migrateDB(d.db, config.DBTypeSQLite, migrate.Up, 0)
}

// MigrateDown migrates only ONE migration down
func (d *sqliteStore) MigrateDown() (int, error) {
	d.logger.Debug("Migrating down")
	return migrateDB(d.db, config.DBTypeSQLite, migrate.Down, 1)
}

func (d *sqliteStore) MigrationStatus() ([]MigrationStatus, error) {
	return migrationStatus(d.db, config.DBTypeSQLite)
}

func (d *sqliteStore) withTx(ctx context.Context, fn func(repo Repository) error) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, "begin transaction")
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	repo := newRepositoryStore(sqliteRepository{queries: reposqlite.New(tx)}, d.logger)
	if err = fn(repo); err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		return errors.Wrap(err, "commit transaction")
	}

	committed = true
	return nil
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
