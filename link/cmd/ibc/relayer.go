package main

import (
	"log/slog"

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

func relayerRun(_ *cobra.Command, _ []string) error {
	cfg, err := setupHomeWithConfig()
	if err != nil {
		return err
	}

	app, err := bootstrap.BuildRelayer(cfg)
	if err != nil {
		return err
	}

	slog.Info("Starting relayer")

	if err := app.Server.Start(); err != nil {
		app.Logger.Error("Failed to start relayer server", "error", err)
		return err
	}

	graceful.AddCallback(app.Server.Stop)

	// blocking
	return graceful.WaitShutdown()
}
