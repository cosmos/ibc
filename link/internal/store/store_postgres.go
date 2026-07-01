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

	repopostgres "github.com/cosmos/ibc/link/internal/store/repository/postgres"
	migrate "github.com/rubenv/sql-migrate"
)

type postgresStore struct {
	// connection pool
	pool *pgxpool.Pool

	// *sql.DB wrapper for migrations
	sqlWrapper *sql.DB

	*repositoryStore

	logger *slog.Logger
}

var (
	_ Store            = (*postgresStore)(nil)
	_ transactionStore = (*postgresStore)(nil)
)

// NewPostgres creates a new Store instance with pgx connection pool.
// Context must be long-lived. URL example:
// "postgres://username:password@localhost:5432/database_name"
func NewPostgres(ctx context.Context, url string) (Store, error) {
	poolConfig, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, errors.Wrap(err, "parse config")
	}

	return NewPostgresWithConfig(ctx, poolConfig, true)
}

// NewPostgresWithConfig creates a new Store instance based on pgxpool.Config
// Allows to modify the config before creation.
func NewPostgresWithConfig(ctx context.Context, config *pgxpool.Config, ping bool) (Store, error) {
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, errors.Wrap(err, "create pool")
	}

	logger := slog.With("module", "database")
	repo := postgresRepository{queries: repopostgres.New(pool)}

	db := &postgresStore{
		pool:            pool,
		sqlWrapper:      stdlib.OpenDBFromPool(pool),
		repositoryStore: newRepositoryStore(repo, logger),
		logger:          logger,
	}

	if ping {
		if err := db.Ping(ctx); err != nil {
			return nil, err
		}
	}

	return db, nil
}

func (d *postgresStore) Close() error {
	if err := d.sqlWrapper.Close(); err != nil {
		return errors.Wrap(err, "close sql wrapper")
	}

	d.pool.Close()

	return nil
}

func (d *postgresStore) Ping(ctx context.Context) error {
	ctxPing, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	start := time.Now()

	if err := d.pool.Ping(ctxPing); err != nil {
		d.logger.Error("Unable to ping database", "err", err, "elapsed", time.Since(start).String())
		return errors.Wrap(err, "unable to ping database")
	}

	d.logger.Info("Ping", "elapsed", time.Since(start).String())

	return nil
}

// MigrateUp migrates ALL available migrations
func (d *postgresStore) MigrateUp() (int, error) {
	d.logger.Debug("Migrating up")
	return migrateDB(d.sqlWrapper, config.DBTypePostgres, migrate.Up, 0)
}

// MigrateDown migrates only ONE migration down
func (d *postgresStore) MigrateDown() (int, error) {
	d.logger.Debug("Migrating down")
	return migrateDB(d.sqlWrapper, config.DBTypePostgres, migrate.Down, 1)
}

func (d *postgresStore) MigrationStatus() ([]MigrationStatus, error) {
	return migrationStatus(d.sqlWrapper, config.DBTypePostgres)
}

func (d *postgresStore) withTx(ctx context.Context, fn func(repo Repository) error) error {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return errors.Wrap(err, "begin transaction")
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	repo := newRepositoryStore(postgresRepository{queries: repopostgres.New(tx)}, d.logger)
	if err = fn(repo); err != nil {
		return err
	}

	if err = tx.Commit(ctx); err != nil {
		return errors.Wrap(err, "commit transaction")
	}

	committed = true
	return nil
}
