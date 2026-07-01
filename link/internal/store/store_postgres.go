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

	queries *repopostgres.Queries

	logger *slog.Logger
}

var _ Database = (*postgresStore)(nil)

func newPostgres(ctx context.Context, url string) (*postgresStore, error) {
	poolConfig, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, pkgerrors.Wrap(err, "parse config")
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, pkgerrors.Wrap(err, "create pool")
	}

	logger := slog.With("module", "database")

	db := &postgresStore{
		pool:       pool,
		sqlWrapper: stdlib.OpenDBFromPool(pool),
		queries:    repopostgres.New(pool),
		logger:     logger,
	}

	if err := db.Ping(ctx); err != nil {
		_ = db.Close()
		return nil, err
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

func (d *postgresStore) MigrationStatus() ([]MigrationStatus, error) {
	return migrationStatus(d.sqlWrapper, config.DBTypePostgres)
}

func (d *postgresStore) GetRelaySubmission(
	ctx context.Context,
	key RelaySubmissionKey,
) (*RelaySubmission, error) {
	d.logger.Debug("GetRelaySubmission", "chainID", key.ChainID, "txHash", key.TxHash)

	if err := key.Validate(); err != nil {
		return nil, err
	}

	entry, err := d.queries.GetRelaySubmission(ctx, key.ChainID, key.TxHash)
	if err != nil {
		return nil, pkgerrors.Wrap(errNormalize(err), "get relay submission")
	}

	return newRelaySubmission(entry.ID, entry.SourceChainID, entry.SourceTxHash, entry.CreatedAt), nil
}

func (d *postgresStore) UpsertRelaySubmission(ctx context.Context, key RelaySubmissionKey) error {
	d.logger.Debug("UpsertRelaySubmission", "chainID", key.ChainID, "txHash", key.TxHash)

	if err := key.Validate(); err != nil {
		return err
	}

	if err := d.queries.UpsertRelaySubmission(ctx, key.ChainID, key.TxHash); err != nil {
		return pkgerrors.Wrap(err, "upsert relay submission")
	}

	return nil
}
