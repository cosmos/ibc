// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

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

// useIFT is the shared "ift" subcommand name, factored out to satisfy
// goconst across cmd/ibc.
const useIFT = "ift"

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
		cmdTx,
	)

	cmdConfig.AddCommand(cmdConfigNew, cmdConfigValidate, cmdConfigAddChain)
	cmdConfigNew.Flags().BoolVar(&flagConfigNewOut, "out", false, "output the config to stdout")
	cmdConfigValidate.Flags().BoolVar(&flagConfigValidateLive, "live", false, "extra validation checks")
	cmdConfigValidate.Flags().
		BoolVar(&flagConfigValidateStrict, "strict", false, "fail on unknown fields in the config file")
	cmdConfigAddChain.Flags().StringVar(&flagConfigAddChainID, "chain-id", "", "chain ID")
	cmdConfigAddChain.Flags().StringVar(&flagConfigAddChainRPC, "rpc", "", "chain RPC URL")
	cmdConfigAddChain.Flags().
		StringVar(&flagConfigAddChainRouter, "router", "", "ics26Router address (default: left blank, fill in via render-config)")
	cmdConfigAddChain.Flags().
		StringVar(&flagConfigAddChainDeployer, "deployer", "", "signer alias used by ibc deploy for this chain")
	for _, req := range []string{"chain-id", "rpc"} {
		_ = cmdConfigAddChain.MarkFlagRequired(req)
	}

	// Keys commands
	cmdKeys.AddCommand(cmdKeysNew, cmdKeysShow, cmdKeysImport, cmdKeysList)
	cmdKeysShow.Flags().BoolVarP(&flagKeysShowPrivate, "private", "", false, "show private key")
	cmdKeysImport.Flags().StringVar(&flagKeysImportPrivateKey, "private-key", "", "hex-encoded private key")
	for _, c := range []*cobra.Command{cmdKeysNew, cmdKeysImport} {
		c.Flags().
			BoolVar(&flagKeysPopulateConfig, "populate-config", false, "append the resulting key as a signers entry in the config file")
	}

	// Relayer commands
	cmdRelayer.AddCommand(cmdRelayerRun, cmdRelayerRelay, cmdRelayerPackets)
	cmdRelayerRun.Flags().BoolVarP(&flagRelayerNoMigrate, "no-migrate", "", false, "skip database migrations")
	for _, c := range []*cobra.Command{cmdRelayerRelay, cmdRelayerPackets} {
		c.Flags().StringVar(&flagRelayerHost, "host", "", "dial this address instead of resolving from config")
		c.Flags().StringVar(&flagRelayerTxHash, "tx-hash", "", "source transaction hash")
		c.Flags().StringVar(&flagRelayerSourceChainID, "chain-id", "", "source chain id")
	}

	_ = cmdRelayerRelay.MarkFlagRequired("tx-hash")
	_ = cmdRelayerRelay.MarkFlagRequired("chain-id")

	cmdRelayerPackets.Flags().
		StringVar(&flagRelayerPacketsDestChainID, "destination-chain-id", "", "destination chain id")
	cmdRelayerPackets.Flags().
		StringVar(&flagRelayerPacketsSrcClientID, "source-client-id", "", "source client id")
	cmdRelayerPackets.Flags().
		StringVar(&flagRelayerPacketsDestClientID, "destination-client-id", "", "destination client id")
	cmdRelayerPackets.Flags().
		StringVar(&flagRelayerPacketsState, "state", "", "relay state ("+strings.Join(packetStateNames(), ", ")+")")
	cmdRelayerPackets.Flags().
		Uint64Var(&flagRelayerPacketsSequence, "sequence", 0, "packet sequence number")
	cmdRelayerPackets.Flags().
		Uint32Var(&flagRelayerPacketsLimit, "limit", 0, "maximum packets to return (default 100, max 1000)")
	cmdRelayerPackets.Flags().
		StringVar(&flagRelayerPacketsCursor, "cursor", "", "next_cursor from a previous response, to resume paging")
	cmdRelayerPackets.Flags().
		BoolVar(&flagRelayerPacketsAll, "all", false, "follow every page and print the combined result")

	// Attestor commands
	cmdAttestor.AddCommand(cmdAttestorRun, cmdAttestorInfo, cmdAttestorLatestHeight, cmdAttestorStateAttestation)
	for _, c := range []*cobra.Command{cmdAttestorInfo, cmdAttestorLatestHeight, cmdAttestorStateAttestation} {
		c.Flags().StringVar(&flagAttestorHost, "host", "", "dial this address instead of resolving from config")
	}
	cmdAttestorStateAttestation.Flags().Uint64Var(&flagAttestorHeight, "height", 0, "height to attest")

	// Query commands
	cmdQuery.AddCommand(cmdQueryIFT)
	cmdQueryIFT.AddCommand(cmdQueryIFTBalance)
	qpf := cmdQueryIFT.PersistentFlags()
	qpf.StringVar(&flagQueryIFTChain, "chain", "", "chain ID the IFT token is deployed on")
	qpf.StringVar(&flagQueryIFTAddress, "ift", "", "IFT token address")
	for _, req := range []string{"chain", useIFT} {
		_ = cmdQueryIFT.MarkPersistentFlagRequired(req)
	}
	cmdQueryIFTBalance.Flags().
		StringVar(&flagQueryIFTAccount, "address", "", "account address, or a configured signer alias")
	_ = cmdQueryIFTBalance.MarkFlagRequired("address")

	// Migrate commands
	cmdMigrate.AddCommand(cmdMigrateUp, cmdMigrateDown, cmdMigrateStatus)

	// Deploy commands
	cmdDeploy.AddCommand(
		cmdDeployCore, cmdDeployClient,
		cmdDeployStatus, cmdDeployShow, cmdDeployRenderConfig,
		cmdDeployGMP, cmdDeployIFT, cmdDeployIFTBridge,
	)
	dpf := cmdDeploy.PersistentFlags()
	dpf.StringVar(&flagDeployManifestDir, "manifest-dir", "deployments", "manifest directory relative to home")
	dpf.StringVar(&flagDeployDeployer, "deployer", "", "signer alias override for deployment transactions")
	dpf.StringVar(&flagDeployChain, "chain", "", "chain ID for the chain being deployed to")
	dpf.BoolVar(&flagDeployDryRun, "dry-run", false, "print the step plan without submitting transactions")
	dpf.BoolVar(&flagDeployYes, "yes", false, "skip confirmation prompts")

	cmdDeployClient.Flags().
		StringVar(&flagDeployCounterparty, "counterparty-chain", "", "counterparty chain id the client tracks")
	_ = cmdDeployClient.MarkFlagRequired("counterparty-chain")
	cmdDeployClient.Flags().StringVar(&flagDeployClientType, "type", deploy.ClientTypeAttestation, "light client type")
	cmdDeployClient.Flags().
		StringSliceVar(&flagDeployAttestors, "attestors", nil,
			"attestors for the new client: addresses, attestation names, or signer aliases (default: configured attestations for the tracked chain)")
	cmdDeployClient.Flags().Uint8Var(&flagDeployThreshold, "threshold", 1, "attestation signature threshold")
	cmdDeployClient.Flags().
		StringVar(&flagDeployClientID, "client-id", "", "client id (default: link-<a>-<b>, chain ids sorted)")
	cmdDeployClient.Flags().
		StringVar(&flagDeployCounterpartyCID, "counterparty-client-id", "", "counterparty's client id (default: link-<a>-<b>, chain ids sorted)")
	cmdDeployClient.Flags().
		Uint64Var(&flagDeployHeight, "height", 0, "initial trusted height (default: counterparty head)")
	cmdDeployClient.Flags().
		Uint64Var(&flagDeployTimestamp, "timestamp", 0, "initial trusted timestamp seconds (default: counterparty head)")

	// IFT commands
	cmdDeployIFT.Flags().StringVar(&flagDeployIFTName, "name", "", "ERC20 token name")
	cmdDeployIFT.Flags().StringVar(&flagDeployIFTSymbol, "symbol", "", "ERC20 token symbol (need not be unique)")
	cmdDeployIFT.Flags().StringVar(&flagDeployIFTOwner, "owner", "", "token owner address (default: deployer)")
	_ = cmdDeployIFT.MarkFlagRequired("name")
	_ = cmdDeployIFT.MarkFlagRequired("symbol")

	cmdDeployIFTBridge.Flags().StringVar(&flagDeployBridgeChainA, "chain-a", "", "first chain id")
	cmdDeployIFTBridge.Flags().StringVar(&flagDeployBridgeIFTA, "ift-a", "", "IFT token address on chain A")
	cmdDeployIFTBridge.Flags().StringVar(&flagDeployBridgeChainB, "chain-b", "", "second chain id")
	cmdDeployIFTBridge.Flags().StringVar(&flagDeployBridgeIFTB, "ift-b", "", "IFT token address on chain B")
	for _, req := range []string{"chain-a", "ift-a", "chain-b", "ift-b"} {
		_ = cmdDeployIFTBridge.MarkFlagRequired(req)
	}
	cmdDeployIFTBridge.Flags().
		StringVar(&flagDeployBridgeClientID, "client-id", "", "client id the bridge relays over (default: link-<a>-<b>)")
	cmdDeployIFTBridge.Flags().
		StringVar(&flagDeployBridgeCtorA, "send-call-constructor-a", "",
			"send call constructor address on chain A (default: deploy or reuse the EVM constructor)")
	cmdDeployIFTBridge.Flags().
		StringVar(&flagDeployBridgeCtorB, "send-call-constructor-b", "",
			"send call constructor address on chain B (default: deploy or reuse the EVM constructor)")

	// Tx commands
	cmdTx.AddCommand(cmdTxIFT)
	cmdTxIFT.AddCommand(cmdTxIFTMint, cmdTxIFTSend)
	tpf := cmdTxIFT.PersistentFlags()
	tpf.StringVar(&flagTxIFTChain, "chain", "", "chain ID the IFT token is deployed on")
	tpf.StringVar(&flagTxIFTAddress, "ift", "", "IFT token address")
	tpf.StringVar(&flagTxIFTFrom, "from", "", "signer alias to submit the transaction with")
	for _, req := range []string{"chain", useIFT, "from"} {
		_ = cmdTxIFT.MarkPersistentFlagRequired(req)
	}
	cmdTxIFTMint.Flags().StringVar(&flagTxIFTTo, "to", "", "recipient address, or a configured signer alias")
	cmdTxIFTMint.Flags().StringVar(&flagTxIFTAmount, "amount", "", "amount to mint, in the token's base unit")
	for _, req := range []string{"to", "amount"} {
		_ = cmdTxIFTMint.MarkFlagRequired(req)
	}
	cmdTxIFTSend.Flags().StringVar(&flagTxIFTClientID, "client-id", "", "client id the bridge is registered for")
	cmdTxIFTSend.Flags().
		StringVar(&flagTxIFTTo, "to", "", "receiver address on the counterparty chain, or a configured signer alias")
	cmdTxIFTSend.Flags().StringVar(&flagTxIFTAmount, "amount", "", "amount to send, in the token's base unit")
	cmdTxIFTSend.Flags().
		DurationVar(&flagTxIFTTimeout, "timeout", 15*time.Minute, "relative send timeout")
	for _, req := range []string{"client-id", "to", "amount"} {
		_ = cmdTxIFTSend.MarkFlagRequired(req)
	}
}
