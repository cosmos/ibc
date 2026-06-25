// Package main is the entrypoint for the ibc-link binary
package main

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/cosmos/ibc/link/internal/config"
)

// global globalFlags, loaded in config.DeclarePersistentFlags()
var globalFlags = config.DefaultFlagSet()

var rootCmd = &cobra.Command{
	Use:   "ibc",
	Short: "IBC Link",
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// single init() for binding all commands to rootCmd
func init() {
	// setup global flags
	config.DeclarePersistentFlags(rootCmd, &globalFlags)

	rootCmd.AddCommand(
		cmdConfig,
		cmdRelayer,
		cmdAttestor,
		cmdQuery,
	)

	// Config commands
	cmdConfig.AddCommand(cmdConfigNew, cmdConfigValidate)
	cmdConfigValidate.Flags().BoolVarP(&flagValidateLive, "live", "", false, "extra validation checks")

	// Relayer commands
	cmdRelayer.AddCommand(cmdRelayerRun)

	// Attestor commands
	cmdAttestor.AddCommand(cmdAttestorRun)

	// Query commands
	// todo
}
