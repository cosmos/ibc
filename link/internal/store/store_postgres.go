package store

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/config"

	repopostgres "github.com/cosmos/ibc/link/internal/store/repository/postgres"
	migrate "github.com/rubenv/sql-migrate"
)

// PostgresDB is a wrapper around the postgres database.
type PostgresDB struct {
	// connection pool
	pool *pgxpool.Pool

	// *sql.DB wrapper for migrations
	sqlWrapper *sql.DB

	*repositoryStore

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

	return NewPostgresWithConfig(ctx, poolConfig)
}

// NewPostgresWithConfig creates a new PostgresDB instance based on pgxpool.Config
// Allows to modify the config before creation.
func NewPostgresWithConfig(ctx context.Context, config *pgxpool.Config) (*PostgresDB, error) {
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, errors.Wrap(err, "create pool")
	}

	logger := slog.With("module", "database")
	repo := postgresRepository{queries: repopostgres.New(pool)}

	return &PostgresDB{
		pool:            pool,
		sqlWrapper:      stdlib.OpenDBFromPool(pool),
		repositoryStore: newRepositoryStore(repo, logger),
		logger:          logger,
	}, nil
}

func (db *PostgresDB) Close() error {
	if err := db.sqlWrapper.Close(); err != nil {
		return errors.Wrap(err, "close sql wrapper")
	}

	db.pool.Close()

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
