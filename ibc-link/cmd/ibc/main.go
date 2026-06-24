// Package main is the entrypoint for the ibc-link binary
package main

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/cosmos/ibc/ibc-link/config"
)

// todo: commands: config validate, config create

// global flags, loaded in config.DeclarePersistentFlags()
var flags = config.DefaultFlagSet()

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
	config.DeclarePersistentFlags(rootCmd, &flags)

	rootCmd.AddCommand(cmdRelayer)
	cmdRelayer.AddCommand(cmdRelayerRun)
}
