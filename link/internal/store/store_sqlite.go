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

const sqliteInMemory = ":memory:"

type sqliteStore struct {
	db *sql.DB

	queries *reposqlite.Queries

	logger *slog.Logger
}

var _ Database = (*sqliteStore)(nil)

func defaultSQLiteConnOptions() map[string]string {
	return map[string]string{
		"_pragma": "journal_mode(WAL)", // Write-Ahead Logging mode
		"mode":    "rwc",               // read, write, create file
	}
}

func newSQLite(path string) (*sqliteStore, error) {
	return newSQLiteWithOptions(path, defaultSQLiteConnOptions())
}

func newSQLiteWithOptions(path string, connectionOpts map[string]string) (*sqliteStore, error) {
	if path == "" {
		return nil, errors.New("path is required")
	}

	logger := slog.With("module", "database")

	if path == sqliteInMemory {
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

func newSqliteStore(db *sql.DB, logger *slog.Logger) *sqliteStore {
	return &sqliteStore{
		db:      db,
		queries: reposqlite.New(db),
		logger:  logger,
	}
}

func (d *sqliteStore) Close() error {
	return d.db.Close()
}

func (d *sqliteStore) Ping(ctx context.Context) error {
	return d.db.PingContext(ctx)
}

// MigrateUp migrates ALL available migrations
func (d *sqliteStore) MigrateUp(ctx context.Context) (int, error) {
	d.logger.Debug("Migrating up")
	return migrateDB(ctx, d.db, config.DBTypeSQLite, migrate.Up, 0)
}

// MigrateDown migrates only ONE migration down
func (d *sqliteStore) MigrateDown(ctx context.Context) (int, error) {
	d.logger.Debug("Migrating down")
	return migrateDB(ctx, d.db, config.DBTypeSQLite, migrate.Down, 1)
}

func (d *sqliteStore) MigrationStatus() ([]MigrationStatus, error) {
	return migrationStatus(d.db, config.DBTypeSQLite)
}

func (d *sqliteStore) GetRelaySubmission(
	ctx context.Context,
	key RelaySubmissionKey,
) (*RelaySubmission, error) {
	d.logger.Debug("GetRelaySubmission", "chainID", key.ChainID, "txHash", key.TxHash)

	if err := key.Validate(); err != nil {
		return nil, err
	}

	entry, err := d.queries.GetRelaySubmission(ctx, key.ChainID, key.TxHash)
	if err != nil {
		return nil, errors.Wrap(errNormalize(err), "get relay submission")
	}

	return newRelaySubmission(entry.ID, entry.SourceChainID, entry.SourceTxHash, entry.CreatedAt), nil
}

func (d *sqliteStore) UpsertRelaySubmission(ctx context.Context, key RelaySubmissionKey) error {
	d.logger.Debug("UpsertRelaySubmission", "chainID", key.ChainID, "txHash", key.TxHash)

	if err := key.Validate(); err != nil {
		return err
	}

	if err := d.queries.UpsertRelaySubmission(ctx, key.ChainID, key.TxHash); err != nil {
		return errors.Wrap(err, "upsert relay submission")
	}

	return nil
}

func sqliteURL(path string, connectionOpts map[string]string) (string, error) {
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
