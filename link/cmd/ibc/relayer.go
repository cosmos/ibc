package main

import (
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/cosmos/ibc/link/internal/bootstrap"
	"github.com/cosmos/ibc/link/internal/pkg/graceful"
)

var (
	cmdRelayer = &cobra.Command{
		Use:   "relayer",
		Short: "Relayer commands",
	}

	cmdRelayerRun = &cobra.Command{
		Use:   "run",
		Short: "Run the relayer",
		RunE:  relayerRun,
	}
)

var flagRelayerNoMigrate bool

func relayerRun(_ *cobra.Command, _ []string) error {
	cfg, err := setupHomeWithConfig()
	if err != nil {
		return err
	}

	app, err := bootstrap.BuildRelayer(cfg)
	if err != nil {
		return err
	}

	if flagRelayerNoMigrate {
		app.Logger.Info("--no-migrate flag passed, skipping migrations")
	} else {
		applied, err := app.Store.MigrateUp()
		switch {
		case err != nil:
			return errors.Wrap(err, "failed to migrate database")
		case applied == 0:
			app.Logger.Info("No migrations to apply")
		case applied > 0:
			app.Logger.Info("Migrated database", "migrations_applied", applied)
		}
	}

	app.Logger.Info("Starting relayer")

	if err := app.Server.Start(); err != nil {
		app.Logger.Error("Failed to start relayer server", "error", err)
		return err
	}

	if err := app.RelayerService.Start(); err != nil {
		app.Logger.Error("Failed to start relayer dispatch loop", "error", err)
		return err
	}

	graceful.AddCallback(app.Store.Close)
	graceful.AddCallback(app.RelayerService.Stop)
	graceful.AddCallback(app.Server.Stop)

	// blocking
	return graceful.WaitShutdown()
}
