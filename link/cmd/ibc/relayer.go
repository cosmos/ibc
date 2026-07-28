package main

import (
	"encoding/json"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/cosmos/ibc/link/cmd/relayercmd"
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

func relayerRun(cmd *cobra.Command, _ []string) error {
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
		applied, migrateErr := app.Store.MigrateUp()
		switch {
		case migrateErr != nil:
			return errors.Wrap(migrateErr, "failed to migrate database")
		case applied == 0:
			app.Logger.Info("No migrations to apply")
		case applied > 0:
			app.Logger.Info("Migrated database", "migrations_applied", applied)
		}
	}

	app.Logger.Info("Starting relayer")

	address, err := app.Server.Start()
	if err != nil {
		app.Logger.Error("Failed to start relayer server", "error", err)
		return err
	}

	if err := app.RelayerService.Start(); err != nil {
		app.Logger.Error("Failed to start relayer dispatch loop", "error", err)
		_ = app.Server.Stop()
		return err
	}

	connected := make([]string, 0, len(cfg.Chains))
	for _, chain := range cfg.Chains {
		connected = append(connected, chain.ChainID)
	}
	if err := json.NewEncoder(cmd.OutOrStdout()).Encode(relayercmd.Readiness{
		Event:           relayercmd.ReadinessEvent,
		ChainsConnected: connected,
		HTTP:            address.String(),
	}); err != nil {
		_ = app.RelayerService.Stop()
		_ = app.Server.Stop()
		return err
	}

	// executes from last to first
	graceful.AddCallback(app.Store.Close)
	graceful.AddCallback(app.Server.Stop)
	graceful.AddCallback(app.RelayerService.Stop)

	// blocking
	return graceful.WaitShutdown()
}
