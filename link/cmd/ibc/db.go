package main

import (
	"fmt"

	"github.com/spf13/cobra"
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
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}

	fmt.Printf("Migration up is not implemented for %s database %q.\n", cfg.DB.Type, cfg.DB.URL)

	return nil
}

func migrateDown(_ *cobra.Command, _ []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}

	fmt.Printf("Migration down is not implemented for %s database %q.\n", cfg.DB.Type, cfg.DB.URL)

	return nil
}

func migrateStatus(_ *cobra.Command, _ []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}

	fmt.Printf("Migration status is not implemented for %s database %q.\n", cfg.DB.Type, cfg.DB.URL)

	return nil
}
