package store

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/store/repository/postgres"

	migrate "github.com/rubenv/sql-migrate"
)

// PostgresDB is a wrapper around the postgres database.
type PostgresDB struct {
	// connection pool
	pool *pgxpool.Pool

	// *sql.DB wrapper for migrations
	sqlWrapper *sql.DB

	// sqlc repository
	repo *postgres.Queries

	logger *slog.Logger
}

var _ Store = (*PostgresDB)(nil)

// NewPostgres creates a new PostgresDB instance with pgx connection pool.
// Context must be long-lived. URL example:
// "postgres://username:password@localhost:5432/database_name"
func NewPostgres(ctx context.Context, url string) (*PostgresDB, error) {
	poolConfig, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, errors.Wrap(err, "parse config")
	}

	return NewPostgresWithConfig(ctx, poolConfig, true)
}

// NewPostgresWithConfig creates a new PostgresDB instance based on pgxpool.Config
// Allows to modify the config before creation.
func NewPostgresWithConfig(ctx context.Context, config *pgxpool.Config, ping bool) (*PostgresDB, error) {
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, errors.Wrap(err, "create pool")
	}
	db := &PostgresDB{
		pool:       pool,
		sqlWrapper: stdlib.OpenDBFromPool(pool),
		repo:       postgres.New(pool),
		logger:     slog.With("module", "database"),
	}

	if ping {
		if err := db.Ping(ctx); err != nil {
			return nil, err
		}
	}

	return db, nil
}

func (db *PostgresDB) Close() error {
	if err := db.sqlWrapper.Close(); err != nil {
		return errors.Wrap(err, "close sql wrapper")
	}

	db.pool.Close()

	return nil
}

func (db *PostgresDB) Ping(ctx context.Context) error {
	ctxPing, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	start := time.Now()

	if err := db.pool.Ping(ctxPing); err != nil {
		db.logger.Error("Unable to ping database", "err", err, "elapsed", time.Since(start).String())
		return errors.Wrap(err, "unable to ping database")
	}

	db.logger.Info("Ping", "elapsed", time.Since(start).String())

	return nil
}

// MigrateUp migrates ALL available migrations
func (db *PostgresDB) MigrateUp() (int, error) {
	db.logger.Debug("Migrating up")
	return migrateDB(db.sqlWrapper, config.DBTypePostgres, migrate.Up, 0)
}

// MigrateDown migrates only ONE migration down
func (db *PostgresDB) MigrateDown() (int, error) {
	db.logger.Debug("Migrating down")
	return migrateDB(db.sqlWrapper, config.DBTypePostgres, migrate.Down, 1)
}

func (db *PostgresDB) MigrationStatus() ([]MigrationStatus, error) {
	return migrationStatus(db.sqlWrapper, config.DBTypePostgres)
}

func (db *PostgresDB) GetRelaySubmission(ctx context.Context, chainID string, txHash string) (*RelaySubmission, error) {
	db.logger.Debug("GetRelaySubmission", "chainID", chainID, "txHash", txHash)

	if chainID == "" || txHash == "" {
		return nil, errors.New("chainID and txHash are required")
	}

	entry, err := db.repo.GetRelaySubmission(ctx, chainID, txHash)
	if err != nil {
		return nil, errNormalize(err)
	}

	return &RelaySubmission{
		ID:        entry.ID,
		ChainID:   entry.SourceChainID,
		TxHash:    entry.SourceTxHash,
		CreatedAt: entry.CreatedAt.Time.UTC(),
	}, nil
}

func (db *PostgresDB) UpsertRelaySubmission(ctx context.Context, chainID string, txHash string) error {
	db.logger.Debug("UpsertRelaySubmission", "chainID", chainID, "txHash", txHash)

	if chainID == "" || txHash == "" {
		return errors.New("chainID and txHash are required")
	}

	return db.repo.UpsertRelaySubmission(ctx, chainID, txHash)
}
