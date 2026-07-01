package store

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/cosmos/ibc/link/internal/config"

	repopostgres "github.com/cosmos/ibc/link/internal/store/repository/postgres"
	pkgerrors "github.com/pkg/errors"
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

var _ Database = (*postgresStore)(nil)

func newPostgres(ctx context.Context, url string) (*postgresStore, error) {
	poolConfig, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, pkgerrors.Wrap(err, "parse config")
	}

	return newPostgresWithConfig(ctx, poolConfig, true)
}

func newPostgresWithConfig(ctx context.Context, config *pgxpool.Config, ping bool) (*postgresStore, error) {
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, pkgerrors.Wrap(err, "create pool")
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
			_ = db.Close()
			return nil, err
		}
	}

	return db, nil
}

func (d *postgresStore) Close() error {
	err := d.sqlWrapper.Close()
	d.pool.Close()

	return pkgerrors.Wrap(err, "close sql wrapper")
}

func (d *postgresStore) Ping(ctx context.Context) error {
	ctxPing, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	start := time.Now()

	if err := d.pool.Ping(ctxPing); err != nil {
		d.logger.Error("Unable to ping database", "err", err, "elapsed", time.Since(start).String())
		return pkgerrors.Wrap(err, "unable to ping database")
	}

	d.logger.Info("Ping", "elapsed", time.Since(start).String())

	return nil
}

// MigrateUp migrates ALL available migrations
func (d *postgresStore) MigrateUp(ctx context.Context) (int, error) {
	d.logger.Debug("Migrating up")
	return migrateDB(ctx, d.sqlWrapper, config.DBTypePostgres, migrate.Up, 0)
}

// MigrateDown migrates only ONE migration down
func (d *postgresStore) MigrateDown(ctx context.Context) (int, error) {
	d.logger.Debug("Migrating down")
	return migrateDB(ctx, d.sqlWrapper, config.DBTypePostgres, migrate.Down, 1)
}

func (d *postgresStore) MigrationStatus(ctx context.Context) ([]MigrationStatus, error) {
	return migrationStatus(ctx, d.sqlWrapper, config.DBTypePostgres)
}

func (d *postgresStore) WithTx(ctx context.Context, fn func(repo Repository) error) error {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return pkgerrors.Wrap(err, "begin transaction")
	}

	committed := false
	defer func() {
		if !committed {
			rollbackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = tx.Rollback(rollbackCtx)
		}
	}()

	repo := newRepositoryStore(postgresRepository{queries: repopostgres.New(tx)}, d.logger)
	if err = fn(repo); err != nil {
		return err
	}

	if err = tx.Commit(ctx); err != nil {
		return pkgerrors.Wrap(err, "commit transaction")
	}

	committed = true
	return nil
}
