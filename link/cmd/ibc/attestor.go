package main

import (
	"github.com/spf13/cobra"

	"github.com/cosmos/ibc/link/internal/bootstrap"
	"github.com/cosmos/ibc/link/internal/pkg/graceful"
)

var (
	cmdAttestor = &cobra.Command{
		Use:   "attestor",
		Short: "Attestor commands",
	}

	cmdAttestorRun = &cobra.Command{
		Use:   "run",
		Short: "Run the attestor",
		RunE:  attestorRun,
	}
)

func attestorRun(_ *cobra.Command, _ []string) error {
	cfg, err := setupHomeWithConfig()
	if err != nil {
		return err
	}

	app, err := bootstrap.BuildAttestor(cfg)
	if err != nil {
		return err
	}

	app.Logger.Info("Starting attestor")

	if err := app.Server.Start(); err != nil {
		app.Logger.Error("Failed to start attestor server", "error", err)
		return err
	}

	graceful.AddCallback(app.Server.Stop)

	// blocking
	return graceful.WaitShutdown()
}
