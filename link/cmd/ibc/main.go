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
		cmdQuery,
		cmdMigrate,
		cmdKeys,
	)

	// Config commands
	cmdConfig.AddCommand(cmdConfigNew, cmdConfigValidate)
	cmdConfigValidate.Flags().BoolVarP(&flagConfigValidateLive, "live", "", false, "extra validation checks")
	cmdConfigValidate.Flags().
		BoolVarP(&flagConfigValidateStrict, "strict", "", false, "fail on unknown fields in the config file")

	// Keys commands
	cmdKeys.AddCommand(cmdKeysNew, cmdKeysShow)
	cmdKeysShow.Flags().BoolVarP(&flagKeysShowPrivate, "private", "", false, "show private key")

	// Relayer commands
	cmdRelayer.AddCommand(cmdRelayerRun)
	cmdRelayerRun.Flags().BoolVarP(&flagRelayerNoMigrate, "no-migrate", "", false, "skip database migrations")

	// Query commands
	// todo

	// Migrate commands
	cmdMigrate.AddCommand(cmdMigrateUp, cmdMigrateDown, cmdMigrateStatus)
}
