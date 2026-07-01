package main

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/store"
)

var (
	cmdMigrate = &cobra.Command{
		Use:   "migrate",
		Short: "Database migration commands",
	}

	cmdMigrateUp = &cobra.Command{
		Use:   "up",
		Short: "Migrate DB up",
		RunE:  migrateUp,
	}

	cmdMigrateDown = &cobra.Command{
		Use:   "down",
		Short: "Migrate DB down",
		RunE:  migrateDown,
	}

	cmdMigrateStatus = &cobra.Command{
		Use:   "status",
		Short: "Print migration status",
		RunE:  migrateStatus,
	}
)

func migrateUp(_ *cobra.Command, _ []string) error {
	cfg, db, err := resolveConfigWithStore()
	if err != nil {
		return err
	}

	defer db.Close() //nolint:errcheck

	applied, err := db.MigrateUp()
	if err != nil {
		return err
	}

	return config.PrintJSON(map[string]any{
		"db":      cfg.DB.Label(),
		"applied": applied,
	})
}

func migrateDown(_ *cobra.Command, _ []string) error {
	cfg, db, err := resolveConfigWithStore()
	if err != nil {
		return err
	}
	defer db.Close() //nolint:errcheck

	rolledBack, err := db.MigrateDown()
	if err != nil {
		return err
	}

	return config.PrintJSON(map[string]any{
		"db":         cfg.DB.Label(),
		"rolledBack": rolledBack,
	})
}

func migrateStatus(_ *cobra.Command, _ []string) error {
	cfg, db, err := resolveConfigWithStore()
	if err != nil {
		return err
	}
	defer db.Close() //nolint:errcheck

	statuses, err := db.MigrationStatus()
	if err != nil {
		return err
	}

	return config.PrintJSON(map[string]any{
		"db":         cfg.DB.Label(),
		"migrations": statuses,
	})
}

// util to quickly resolve config + store. Use ONLY for migrations.
func resolveConfigWithStore() (config.Config, store.Store, error) {
	cfg, err := setupHomeWithConfig()
	if err != nil {
		return config.Config{}, nil, err
	}

	db, err := store.NewStore(context.Background(), cfg)
	if err != nil {
		return config.Config{}, nil, err
	}

	return cfg, db, nil
}
