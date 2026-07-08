package store

import (
	"context"
	"database/sql"
	"log/slog"
	"net/url"
	"path/filepath"
	"time"

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

// SqliteDB is a wrapper around the sqlite database.
type SqliteDB struct {
	db     *sql.DB
	repo   *reposqlite.Queries
	logger *slog.Logger
}

var _ Store = (*SqliteDB)(nil)

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

	logger := slog.With("module", "database")

	if path == SqliteInMemory {
		db, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, errors.Wrapf(err, "open sqlite in memory")
		}

		// shared memory database is a single connection
		db.SetMaxOpenConns(1)

		return &SqliteDB{
			db:     db,
			repo:   reposqlite.New(db),
			logger: logger,
		}, nil
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

	return &SqliteDB{
		db:     db,
		repo:   reposqlite.New(db),
		logger: logger,
	}, nil
}

func (db *SqliteDB) Close() error {
	return db.db.Close()
}

func (db *SqliteDB) Ping(ctx context.Context) error {
	return db.db.PingContext(ctx)
}

// MigrateUp migrates ALL available migrations
func (db *SqliteDB) MigrateUp() (int, error) {
	db.logger.Debug("Migrating up")
	return migrateDB(db.db, config.DBTypeSQLite, migrate.Up, 0)
}

// MigrateDown migrates only ONE migration down
func (db *SqliteDB) MigrateDown() (int, error) {
	db.logger.Debug("Migrating down")
	return migrateDB(db.db, config.DBTypeSQLite, migrate.Down, 1)
}

func (db *SqliteDB) MigrationStatus() ([]MigrationStatus, error) {
	return migrationStatus(db.db, config.DBTypeSQLite)
}

func (db *SqliteDB) GetRelayRequest(ctx context.Context, chainID string, txHash string) (*RelayRequest, error) {
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
		CreatedAt: entry.CreatedAt.UTC(),
	}, nil
}

func (db *SqliteDB) UpsertRelayRequest(ctx context.Context, chainID string, txHash string) error {
	db.logger.Debug("UpsertRelayRequest", "chainID", chainID, "txHash", txHash)

	if chainID == "" || txHash == "" {
		return errors.New("chainID and txHash are required")
	}

	return db.repo.UpsertRelayRequest(ctx, chainID, txHash)
}

// CreateTransfer inserts a transfer. Inserting the same packet twice is a noop.
func (db *SqliteDB) CreateTransfer(ctx context.Context, transfer Transfer) error {
	db.logger.Debug(
		"CreateTransfer",
		"chainID", transfer.SourceChainID,
		"clientID", transfer.PacketSourceClientID,
		"sequence", transfer.PacketSequenceNumber,
	)

	if err := transfer.Validate(); err != nil {
		return errors.Wrap(err, "invalid transfer")
	}

	_, err := db.repo.InsertTransfer(ctx, reposqlite.InsertTransferParams{
		SourceChainID:             transfer.SourceChainID,
		DestinationChainID:        transfer.DestinationChainID,
		SourceTxHash:              transfer.SourceTxHash,
		SourceTxTime:              transfer.SourceTxTime.UTC(),
		PacketSequenceNumber:      int64(transfer.PacketSequenceNumber),
		PacketSourceClientID:      transfer.PacketSourceClientID,
		PacketDestinationClientID: transfer.PacketDestinationClientID,
		PacketTimeoutTimestamp:    transfer.PacketTimeoutTimestamp.UTC(),
	})

	return err
}

func (db *SqliteDB) ListTransfersBySourceTx(ctx context.Context, chainID string, txHash string) ([]Transfer, error) {
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
		transfers[i] = transferFromSqlite(row)
	}

	return transfers, nil
}

func transferFromSqlite(row reposqlite.Ibcv2Transfer) Transfer {
	return Transfer{
		ID:        row.ID,
		CreatedAt: row.CreatedAt.UTC(),
		UpdatedAt: row.UpdatedAt.UTC(),

		Status:     TransferStatus(row.Status),
		StatusText: row.StatusText,

		SourceChainID:         row.SourceChainID,
		DestinationChainID:    row.DestinationChainID,
		SourceTxHash:          row.SourceTxHash,
		SourceTxTime:          row.SourceTxTime.UTC(),
		SourceTxFinalizedTime: utcTimePtr(row.SourceTxFinalizedTime),

		PacketSequenceNumber:      uint64(row.PacketSequenceNumber), //nolint:gosec // sequences fit in int64
		PacketSourceClientID:      row.PacketSourceClientID,
		PacketDestinationClientID: row.PacketDestinationClientID,
		PacketTimeoutTimestamp:    row.PacketTimeoutTimestamp.UTC(),

		RecvTxHash:           row.RecvTxHash,
		RecvTxTime:           utcTimePtr(row.RecvTxTime),
		RecvTxRelayerAddress: row.RecvTxRelayerAddress,

		WriteAckTxHash:          row.WriteAckTxHash,
		WriteAckTxTime:          utcTimePtr(row.WriteAckTxTime),
		WriteAckTxFinalizedTime: utcTimePtr(row.WriteAckTxFinalizedTime),
		WriteAckStatus:          writeAckStatusPtr(row.WriteAckStatus),

		AckTxHash:           row.AckTxHash,
		AckTxTime:           utcTimePtr(row.AckTxTime),
		AckTxRelayerAddress: row.AckTxRelayerAddress,

		TimeoutTxHash:           row.TimeoutTxHash,
		TimeoutTxTime:           utcTimePtr(row.TimeoutTxTime),
		TimeoutTxRelayerAddress: row.TimeoutTxRelayerAddress,
	}
}

func utcTimePtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}

	utc := t.UTC()

	return &utc
}

func writeAckStatusPtr(status *string) *WriteAckStatus {
	if status == nil {
		return nil
	}

	s := WriteAckStatus(*status)

	return &s
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
