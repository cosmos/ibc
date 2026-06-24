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
	configPath, err := globalFlags.ConfigPath()
	if err != nil {
		return err
	}

	// todo resolve config or fail

	fmt.Printf("Running relayer with config: %s\n", configPath)

	return nil
}
