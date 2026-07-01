package store

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/config"

	pgx "github.com/jackc/pgx/v5"
)

// Store a unified, database-agnostic API for persistence.
type Store interface {
	Repository
	Migrator

	Ping(ctx context.Context) error
	Close() error
}

// Repository represents database CRUD operations.
type Repository interface {
	GetRelaySubmission(ctx context.Context, chainID string, txHash string) (*RelaySubmission, error)
	UpsertRelaySubmission(ctx context.Context, chainID string, txHash string) error
}

// Migrator abstracts schema migrations
type Migrator interface {
	MigrateUp() (int, error)
	MigrateDown() (int, error)
	MigrationStatus() ([]MigrationStatus, error)
}

// Repository errors
var (
	ErrNotFound = errors.New("not found")
)

// NewStore creates a new Store instance based on the database type.
func NewStore(ctx context.Context, cfg config.Config) (Store, error) {
	switch cfg.DB.Type {
	case config.DBTypeSQLite:
		return NewSqlite(cfg.DB.URL)
	case config.DBTypePostgres:
		return NewPostgres(ctx, cfg.DB.URL)
	default:
		return nil, errors.New("invalid database type")
	}
}

// ValidateConfigLive assumes the config.Config is valid,
// and checks if the database is reachable.
func ValidateConfigLive(cfg config.Config) error {
	// noop, don't create an empty sqlite db
	if cfg.DB.Type == config.DBTypeSQLite {
		return nil
	}

	// contains Ping()
	store, err := NewStore(context.Background(), cfg)
	if err != nil {
		return err
	}

	if errClose := store.Close(); errClose != nil {
		slog.Error("Failed to close database", "err", errClose)
	}

	return nil
}

// RelaySubmission is an pending relay submission.
type RelaySubmission struct {
	ID        int64
	ChainID   string
	TxHash    string
	CreatedAt time.Time
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
