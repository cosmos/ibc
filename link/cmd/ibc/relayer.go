package main

import (
	"encoding/json"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/cosmos/ibc/link/cmd/relayercmd"
	"github.com/cosmos/ibc/link/internal/bootstrap"
	"github.com/cosmos/ibc/link/internal/pkg/graceful"
)

func relayerRun(cmd *cobra.Command, _ []string) error {
	cfg, err := setupHomeWithConfig()
	if err != nil {
		return err
	}

	app, err := bootstrap.BuildRelayer(cfg)
	if err != nil {
		return err
	}

	applied, err := app.Store.MigrateUp()
	switch {
	case err != nil:
		return errors.Wrap(err, "failed to migrate database")
	case applied == 0:
		app.Logger.Info("No migrations to apply")
	default:
		app.Logger.Info("Migrated database", "migrations_applied", applied)
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

	graceful.AddCallback(app.RelayerService.Stop)
	graceful.AddCallback(app.Server.Stop)
	graceful.AddCallback(app.Store.Close)

	// blocking
	return graceful.WaitShutdown()
}
