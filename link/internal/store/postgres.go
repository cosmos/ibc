package store

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/store/repository/postgres"

	migrate "github.com/rubenv/sql-migrate"
)

// PostgresDB is a wrapper around the postgres database.
type PostgresDB struct {
	pool   *pgxpool.Pool
	repo   *postgres.Queries
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

	return &PostgresDB{
		pool:   pool,
		repo:   postgres.New(pool),
		logger: slog.With("module", "database"),
	}, nil
}

func (db *PostgresDB) Close() error {
	db.pool.Close()

	return nil
}

func (db *PostgresDB) MigrateUp() (int, error) {
	return migrateDB(db.asStandardDB(), config.DBTypePostgres, migrate.Up, 0)
}

func (db *PostgresDB) MigrateDown() (int, error) {
	return migrateDB(db.asStandardDB(), config.DBTypePostgres, migrate.Down, 1)
}

func (db *PostgresDB) MigrationStatus() ([]MigrationStatus, error) {
	return migrationStatus(db.asStandardDB(), config.DBTypePostgres)
}

// asStandardDB converts pgxpool.Pool to *sql.DB
func (db *PostgresDB) asStandardDB() *sql.DB {
	return stdlib.OpenDBFromPool(db.pool)
}
