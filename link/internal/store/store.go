package store

import (
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

func NewStore(cfg config.Config) (Store, error) {
	if cfg.DB.Type != config.DBTypeSQLite {
		return nil, errors.New("only sqlite is supported")
	}

	// todo postgres

	return NewSqlite(cfg.DB.URL)
}
