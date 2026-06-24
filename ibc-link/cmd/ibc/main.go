package main

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/cosmos/ibc/ibc-link/config"
)

// todo: commands: config validate, config create

// global flags, updated in setupFlags()
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
