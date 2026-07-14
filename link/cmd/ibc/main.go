package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/cosmos/ibc/link/cmd/configcmd"
	"github.com/cosmos/ibc/link/cmd/relayercmd"
	"github.com/cosmos/ibc/link/cmd/testappcmd"
	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/stub"
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
	cmdConfig := configcmd.NewCommand(configcmd.Handlers{
		New:      configNew,
		Validate: stub.ConfigValidate(&globalFlags),
	})
	cmdRelayer := relayercmd.NewCommand(relayercmd.Handlers{
		Run: stub.RelayerRun(&globalFlags),
	})
	cmdTestApps := testappcmd.NewCommand(testappcmd.Handlers{
		Deploy: stub.TestAppsDeploy(&globalFlags),
	})

	rootCmd.AddCommand(
		cmdConfig,
		cmdRelayer,
		cmdTestApps,
		cmdAttestor,
		cmdQuery,
		cmdMigrate,
		cmdKeys,
	)

	// Keys commands
	cmdKeys.AddCommand(cmdKeysNew, cmdKeysShow)
	cmdKeysShow.Flags().BoolVarP(&flagKeysShowPrivate, "private", "", false, "show private key")

	// Attestor commands
	cmdAttestor.AddCommand(cmdAttestorRun)

	// Query commands
	// todo

	// Migrate commands
	cmdMigrate.AddCommand(cmdMigrateUp, cmdMigrateDown, cmdMigrateStatus)
}
