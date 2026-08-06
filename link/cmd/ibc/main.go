package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/deploy"
	"github.com/cosmos/ibc/link/internal/pkg/logging"
)

// global globalFlags, loaded in config.DeclarePersistentFlags()
var globalFlags = config.DefaultFlagSet()

// useStatus is the shared "status" subcommand name and status-field key,
// factored out to satisfy goconst across cmd/ibc.
const useStatus = "status"

var rootCmd = &cobra.Command{
	Use:   "ibc",
	Short: "IBC Link",
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

	cobra.OnInitialize(func() {
		slog.SetDefault(logging.Default(globalFlags.LogJSON))
	})

	rootCmd.AddCommand(
		cmdConfig,
		cmdRelayer,
		cmdAttestor,
		cmdQuery,
		cmdMigrate,
		cmdKeys,
		cmdDeploy,
	)

	cmdConfig.AddCommand(cmdConfigNew, cmdConfigValidate)
	cmdConfigNew.Flags().BoolVar(&flagConfigNewOut, "out", false, "output the config to stdout")
	cmdConfigValidate.Flags().BoolVar(&flagConfigValidateLive, "live", false, "extra validation checks")
	cmdConfigValidate.Flags().
		BoolVar(&flagConfigValidateStrict, "strict", false, "fail on unknown fields in the config file")

	// Keys commands
	cmdKeys.AddCommand(cmdKeysNew, cmdKeysShow, cmdKeysImport, cmdKeysList)
	cmdKeysShow.Flags().BoolVarP(&flagKeysShowPrivate, "private", "", false, "show private key")
	cmdKeysImport.Flags().StringVar(&flagKeysImportPrivateKey, "private-key", "", "hex-encoded private key")

	// Relayer commands
	cmdRelayer.AddCommand(cmdRelayerRun)
	cmdRelayerRun.Flags().BoolVarP(&flagRelayerNoMigrate, "no-migrate", "", false, "skip database migrations")

	// Attestor commands
	cmdAttestor.AddCommand(cmdAttestorRun)

	// Query commands
	// todo

	// Migrate commands
	cmdMigrate.AddCommand(cmdMigrateUp, cmdMigrateDown, cmdMigrateStatus)

	// Deploy commands
	cmdDeploy.AddCommand(
		cmdDeployCore, cmdDeployClient, cmdDeployConnect,
		cmdDeployStatus,
	)
	dpf := cmdDeploy.PersistentFlags()
	dpf.StringVar(&flagDeployManifestDir, "manifest-dir", "deployments", "manifest directory relative to home")
	dpf.StringVar(&flagDeployDeployer, "deployer", "", "signer alias override for deployment transactions")
	dpf.StringVar(&flagDeployChain, "chain", "", "chain ID for the chain being deployed to")
	dpf.BoolVar(&flagDeployDryRun, "dry-run", false, "print the step plan without submitting transactions")
	dpf.BoolVar(&flagDeployYes, "yes", false, "skip confirmation prompts")

	cmdDeployClient.Flags().
		StringVar(&flagDeployCounterparty, "counterparty-chain", "", "counterparty chain id the client tracks")
	for _, c := range []*cobra.Command{cmdDeployClient, cmdDeployConnect} {
		c.Flags().StringVar(&flagDeployClientType, "type", deploy.ClientTypeAttestation, "light client type")
		c.Flags().StringSliceVar(&flagDeployAttestors, "attestors", nil, "attestor addresses (attestation clients)")
		c.Flags().Uint8Var(&flagDeployThreshold, "threshold", 1, "attestation signature threshold")
	}
	cmdDeployClient.Flags().StringVar(&flagDeployClientID, "client-id", "", "client id (default link-<counterparty>)")
	cmdDeployClient.Flags().
		StringVar(&flagDeployCounterpartyCID, "counterparty-client-id", "", "counterparty's client id (default link-<chain>)")
	cmdDeployClient.Flags().
		Uint64Var(&flagDeployHeight, "height", 0, "initial trusted height (default: counterparty head)")
	cmdDeployClient.Flags().
		Uint64Var(&flagDeployTimestamp, "timestamp", 0, "initial trusted timestamp seconds (default: counterparty head)")
}
