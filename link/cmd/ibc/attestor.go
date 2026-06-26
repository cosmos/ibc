package main

import (
	"fmt"

	"github.com/spf13/cobra"
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

	fmt.Printf("Running attestor on addr %q...\n", cfg.GRPC.ListenAddress)

	return nil
}
