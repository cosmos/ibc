package store

import (
	"context"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/config"
)

// Migrator wraps DB schema migrations
type Migrator interface {
	MigrateUp() (int, error)
	MigrateDown() (int, error)
	MigrationStatus() ([]MigrationStatus, error)
}

// Store a unified, database-agnostic API for persistence.
type Store interface {
	Migrator

	Close() error
}

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
