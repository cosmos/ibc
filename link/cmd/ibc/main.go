package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/cosmos/ibc/link/internal/config"
)

// global globalFlags, loaded in config.DeclarePersistentFlags()
var globalFlags = config.DefaultFlagSet()

var rootCmd = &cobra.Command{
	Use:          "ibc",
	Short:        "IBC Link",
	SilenceUsage: true,
}

func main() {
	os.Exit(runMain())
}

func runMain() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		return 1
	}
	return 0
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
		cmdMigrate,
		cmdKeys,
	)

	cmdConfig.AddCommand(cmdConfigNew, cmdConfigValidate)
	cmdConfigNew.Flags().BoolVar(&flagConfigNewOut, "out", false, "output the config to stdout")
	cmdConfigValidate.Flags().BoolVar(&flagConfigValidateLive, "live", false, "extra validation checks")
	cmdConfigValidate.Flags().
		BoolVar(&flagConfigValidateStrict, "strict", false, "fail on unknown fields in the config file")

	// Keys commands
	cmdKeys.AddCommand(cmdKeysNew, cmdKeysShow)
	cmdKeysShow.Flags().BoolVarP(&flagKeysShowPrivate, "private", "", false, "show private key")

	// Relayer commands
	cmdRelayer.AddCommand(cmdRelayerRun)
	cmdRelayerRun.Flags().BoolVarP(&flagRelayerNoMigrate, "no-migrate", "", false, "skip database migrations")

	// Attestor commands
	cmdAttestor.AddCommand(cmdAttestorRun)

	// Query commands
	// todo

	// Migrate commands
	cmdMigrate.AddCommand(cmdMigrateUp, cmdMigrateDown, cmdMigrateStatus)
}
