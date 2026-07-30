package main

import (
	"encoding/json"

	"github.com/spf13/cobra"

	"github.com/cosmos/ibc/link/internal/bootstrap"
	"github.com/cosmos/ibc/link/internal/pkg/graceful"

	attestorv2 "github.com/cosmos/ibc/link/api/v2/attestor"
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

func attestorRun(cmd *cobra.Command, _ []string) error {
	cfg, err := setupHomeWithConfig()
	if err != nil {
		return err
	}

	app, err := bootstrap.BuildAttestor(cfg)
	if err != nil {
		return err
	}

	app.Logger.Info("Starting attestor")

	address, err := app.Server.Start()
	if err != nil {
		app.Logger.Error("Failed to start attestor server", "err", err)
		return err
	}
	if err := json.NewEncoder(cmd.OutOrStdout()).Encode(attestorv2.ProcessReadiness{
		Event: attestorv2.ProcessReadinessEvent,
		HTTP:  address.String(),
	}); err != nil {
		_ = app.Server.Stop()
		return err
	}

	graceful.AddCallback(app.Server.Stop)

	// blocking
	return graceful.WaitShutdown()
}
