// Command ibc is the e2e stub stand-in for the IBC Link binary.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/cosmos/ibc/link/e2e/stub/internal/exitcode"
	"github.com/cosmos/ibc/link/e2e/stub/internal/relay"
	"github.com/cosmos/ibc/link/e2e/stub/internal/testappdeploy"
	"github.com/cosmos/ibc/link/e2e/stub/internal/validate"
	"github.com/cosmos/ibc/link/internal/config"
)

var globalFlags = config.DefaultFlagSet()

func main() {
	os.Exit(runMain())
}

func runMain() int {
	// SIGINT/SIGTERM cancels ctx so relayer run can drain before exit.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	err := newRoot().ExecuteContext(ctx)
	return exitcode.Of(err)
}

func newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "ibc",
		Short: "IBC Link (stub) — test-only stand-in for the IBC Link binary",
		// Subcommand failures must not dump usage; stdout stays JSON-only.
		SilenceUsage: true,
	}
	config.DeclarePersistentFlags(root, &globalFlags)

	configCmd := group("config", "configuration commands")
	configCmd.AddCommand(validate.Command(&globalFlags))

	testAppsCmd := group("test-apps", "synthetic test application commands")
	testAppsCmd.AddCommand(testappdeploy.Command(&globalFlags))

	relayerCmd := group("relayer", "relayer commands")
	relayerCmd.AddCommand(relay.Command(&globalFlags))

	root.AddCommand(configCmd, testAppsCmd, relayerCmd)
	return root
}

// Unknown subcommand errors (exit 70), not silent help-and-exit-0.
func group(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:          use,
		Short:        short,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
		},
	}
}
