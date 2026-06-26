package main

import (
	"fmt"

	"github.com/spf13/cobra"
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

	fmt.Printf("Running relayer on addr %q...\n", cfg.GRPC.ListenAddress)

	return nil
}
