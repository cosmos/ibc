package store

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
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

	// sqlc repository bound to the pool
	*postgresRepository

	logger *slog.Logger
}

// postgresRepository implements Repository over a pool- or tx-bound sqlc Queries.
type postgresRepository struct {
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

	return NewPostgresWithConfig(ctx, poolConfig, true)
}

// NewPostgresWithConfig creates a new PostgresDB instance based on pgxpool.Config
// Allows to modify the config before creation.
func NewPostgresWithConfig(ctx context.Context, config *pgxpool.Config, ping bool) (*PostgresDB, error) {
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, errors.Wrap(err, "create pool")
	}

	logger := slog.With("module", "database")

	db := &PostgresDB{
		pool:               pool,
		sqlWrapper:         stdlib.OpenDBFromPool(pool),
		postgresRepository: &postgresRepository{repo: postgres.New(pool), logger: logger},
		logger:             logger,
	}

	if ping {
		if err := db.Ping(ctx); err != nil {
			if errClose := db.Close(); errClose != nil {
				db.logger.Error("Failed to close database", "err", errClose)
			}

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

// ExecTx runs fn within a database transaction.
func (db *PostgresDB) ExecTx(ctx context.Context, fn func(Repository) error) error {
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return errors.Wrap(err, "beginning transaction")
	}

	txRepo := &postgresRepository{repo: db.repo.WithTx(tx), logger: db.logger}

	if err := fn(txRepo); err != nil {
		if errRollback := tx.Rollback(ctx); errRollback != nil {
			return errors.Wrapf(err, "rolling back transaction: %s", errRollback)
		}

		return err
	}

	return errors.Wrap(tx.Commit(ctx), "committing transaction")
}

func (db *postgresRepository) GetRelayRequest(
	ctx context.Context,
	chainID string,
	txHash string,
) (*RelayRequest, error) {
	db.logger.Debug("GetRelayRequest", "chainID", chainID, "txHash", txHash)

	if chainID == "" || txHash == "" {
		return nil, errors.New("chainID and txHash are required")
	}

	entry, err := db.repo.GetRelayRequest(ctx, chainID, txHash)
	if err != nil {
		return nil, errNormalize(err)
	}

	return &RelayRequest{
		ID:        entry.ID,
		ChainID:   entry.SourceChainID,
		TxHash:    entry.SourceTxHash,
		CreatedAt: entry.CreatedAt.Time.UTC(),
	}, nil
}

func (db *postgresRepository) CreateRelayRequest(ctx context.Context, chainID string, txHash string) error {
	db.logger.Debug("CreateRelayRequest", "chainID", chainID, "txHash", txHash)

	if chainID == "" || txHash == "" {
		return errors.New("chainID and txHash are required")
	}

	return db.repo.CreateRelayRequest(ctx, chainID, txHash)
}

// CreateTransfer inserts a transfer. Inserting the same packet twice is a noop.
func (db *postgresRepository) CreateTransfer(ctx context.Context, transfer Transfer) error {
	db.logger.Debug(
		"CreateTransfer",
		"chainID", transfer.SourceChainID,
		"clientID", transfer.PacketSourceClientID,
		"sequence", transfer.PacketSequenceNumber,
	)

	if err := transfer.Validate(); err != nil {
		return errors.Wrap(err, "invalid transfer")
	}

	_, err := db.repo.InsertTransfer(ctx, postgres.InsertTransferParams{
		SourceChainID:             transfer.SourceChainID,
		DestinationChainID:        transfer.DestinationChainID,
		SourceTxHash:              transfer.SourceTxHash,
		SourceTxTime:              pgTimestamp(transfer.SourceTxTime),
		PacketSequenceNumber:      int64(transfer.PacketSequenceNumber),
		PacketSourceClientID:      transfer.PacketSourceClientID,
		PacketDestinationClientID: transfer.PacketDestinationClientID,
		PacketTimeoutTimestamp:    pgTimestamp(transfer.PacketTimeoutTimestamp),
	})

	return err
}

func (db *postgresRepository) ListTransfersBySourceTx(
	ctx context.Context,
	chainID string,
	txHash string,
) ([]Transfer, error) {
	db.logger.Debug("ListTransfersBySourceTx", "chainID", chainID, "txHash", txHash)

	if chainID == "" || txHash == "" {
		return nil, errors.New("chainID and txHash are required")
	}

	rows, err := db.repo.ListTransfersBySourceTx(ctx, chainID, txHash)
	if err != nil {
		return nil, errNormalize(err)
	}

	transfers := make([]Transfer, len(rows))
	for i, row := range rows {
		transfers[i] = transferFromPostgres(row)
	}

	return transfers, nil
}

func transferFromPostgres(row postgres.Ibcv2Transfer) Transfer {
	return Transfer{
		ID:        row.ID,
		CreatedAt: row.CreatedAt.Time.UTC(),
		UpdatedAt: row.UpdatedAt.Time.UTC(),

		Status:     TransferStatus(row.Status),
		StatusText: row.StatusText,

		SourceChainID:         row.SourceChainID,
		DestinationChainID:    row.DestinationChainID,
		SourceTxHash:          row.SourceTxHash,
		SourceTxTime:          row.SourceTxTime.Time.UTC(),
		SourceTxFinalizedTime: pgTimePtr(row.SourceTxFinalizedTime),

		PacketSequenceNumber:      uint64(row.PacketSequenceNumber), //nolint:gosec // sequences fit in int64
		PacketSourceClientID:      row.PacketSourceClientID,
		PacketDestinationClientID: row.PacketDestinationClientID,
		PacketTimeoutTimestamp:    row.PacketTimeoutTimestamp.Time.UTC(),

		RecvTxHash:           row.RecvTxHash,
		RecvTxTime:           pgTimePtr(row.RecvTxTime),
		RecvTxRelayerAddress: row.RecvTxRelayerAddress,

		WriteAckTxHash:          row.WriteAckTxHash,
		WriteAckTxTime:          pgTimePtr(row.WriteAckTxTime),
		WriteAckTxFinalizedTime: pgTimePtr(row.WriteAckTxFinalizedTime),
		WriteAckStatus:          pgWriteAckStatusPtr(row.WriteAckStatus),

		AckTxHash:           row.AckTxHash,
		AckTxTime:           pgTimePtr(row.AckTxTime),
		AckTxRelayerAddress: row.AckTxRelayerAddress,

		TimeoutTxHash:           row.TimeoutTxHash,
		TimeoutTxTime:           pgTimePtr(row.TimeoutTxTime),
		TimeoutTxRelayerAddress: row.TimeoutTxRelayerAddress,
	}
}

func pgTimestamp(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func pgTimePtr(ts pgtype.Timestamptz) *time.Time {
	if !ts.Valid {
		return nil
	}

	t := ts.Time.UTC()

	return &t
}

func pgWriteAckStatusPtr(status postgres.NullIbcv2WriteAckStatus) *WriteAckStatus {
	if !status.Valid {
		return nil
	}

	s := WriteAckStatus(status.Ibcv2WriteAckStatus)

	return &s
}
