package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/cosmos/ibc/link/internal/config"

	pgx "github.com/jackc/pgx/v5"
)

// Store a unified, database-agnostic API for persistence.
type Store interface {
	GetRelaySubmission(ctx context.Context, key RelaySubmissionKey) (*RelaySubmission, error)
	UpsertRelaySubmission(ctx context.Context, key RelaySubmissionKey) error

	Ping(ctx context.Context) error
	Close() error
}

// Database is the full database subsystem, including runtime store behavior
// and schema migrations.
type Database interface {
	Store
	Migrator
}

// Migrator abstracts schema migrations
type Migrator interface {
	MigrateUp(ctx context.Context) (int, error)
	MigrateDown(ctx context.Context) (int, error)
	MigrationStatus() ([]MigrationStatus, error)
}

// Store errors
var (
	ErrNotFound       = errors.New("not found")
	ErrMissingChainTx = errors.New("chainID and txHash are required")
)

// NewDatabase creates a new Database instance based on the database type.
func NewDatabase(ctx context.Context, cfg config.Config) (Database, error) {
	switch cfg.DB.Type {
	case config.DBTypeSQLite:
		return newSQLite(cfg.DB.URL)
	case config.DBTypePostgres:
		return newPostgres(ctx, cfg.DB.URL)
	default:
		return nil, errors.New("invalid database type")
	}
}

// ValidateConfigLive assumes the config.Config is valid,
// and checks if the database is reachable.
func ValidateConfigLive(ctx context.Context, cfg config.Config) error {
	// noop, don't create an empty sqlite db
	if cfg.DB.Type == config.DBTypeSQLite {
		return nil
	}

	db, err := NewDatabase(ctx, cfg)
	if err != nil {
		return err
	}

	return db.Close()
}

// RelaySubmission is a pending relay submission.
type RelaySubmission struct {
	ID        int64
	ChainID   string
	TxHash    string
	CreatedAt time.Time
}

// RelaySubmissionKey identifies a relay submission by source chain and transaction hash.
type RelaySubmissionKey struct {
	ChainID string
	TxHash  string
}

func (k RelaySubmissionKey) Validate() error {
	if k.ChainID == "" || k.TxHash == "" {
		return ErrMissingChainTx
	}

	return nil
}

// cast db-specific errors to repository errors
func errNormalize(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, sql.ErrNoRows), errors.Is(err, pgx.ErrNoRows):
		return ErrNotFound
	default:
		return err
	}
}

func newRelaySubmission(id int64, chainID string, txHash string, createdAt time.Time) *RelaySubmission {
	return &RelaySubmission{
		ID:        id,
		ChainID:   chainID,
		TxHash:    txHash,
		CreatedAt: createdAt.UTC(),
	}
}
