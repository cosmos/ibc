// Command ibc is the stub stand-in for the IBC Link binary. The harness execs it and parses
// its public output. Stable JSON goes to stdout, human logs to stderr, and the process exits with the
// sysexits code for the outcome.
//
// The black-box wall: everything under internal/ is the stub's guts, which Go's import rule keeps the
// harness/** packages from reaching. The boundary is the CLI, YAML config, and JSON output.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/cosmos/ibc/link/e2e/stub/internal/deploy"
	"github.com/cosmos/ibc/link/e2e/stub/internal/exitcode"
	"github.com/cosmos/ibc/link/e2e/stub/internal/relay"
	"github.com/cosmos/ibc/link/e2e/stub/internal/validate"
	"github.com/cosmos/ibc/link/internal/config"
)

var globalFlags = config.DefaultFlagSet()

func main() {
	os.Exit(runMain())
}

func runMain() int {
	// Cancel the command context on SIGINT/SIGTERM so a long-running command (the relayer daemon) shuts
	// down gracefully; the short one-shot commands inherit a cancellable context for their per-RPC timeouts.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	err := newRoot().ExecuteContext(ctx)
	return exitcode.Of(err)
}

func newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "ibc",
		Short: "IBC Link (stub) — test-only stand-in for the IBC Link binary",
		// A failing subcommand is a logic error, not a usage error: don't dump usage on every failure.
		// Errors still print to stderr (SilenceErrors stays false), keeping stdout JSON-only.
		SilenceUsage: true,
	}
	config.DeclarePersistentFlags(root, &globalFlags)

	configCmd := group("config", "configuration commands")
	configCmd.AddCommand(validate.Command(&globalFlags))

	relayerCmd := group("relayer", "relayer commands")
	relayerCmd.AddCommand(relay.Command(&globalFlags))

	root.AddCommand(configCmd, deploy.Command(&globalFlags), relayerCmd)
	return root
}

// group builds a parent command that only routes to its subcommands. With no args it prints help and
// exits 0; given an unrecognized subcommand it errors (exit 70) instead of silently printing help and
// exiting 0 — so an unknown/typo'd command is a loud failure, matching the CLI wire.
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
