// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"encoding/json"

	"connectrpc.com/connect"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/proto"

	relayerv2 "github.com/cosmos/ibc/link/api/v2/relayer"
	"github.com/cosmos/ibc/link/internal/bootstrap"
	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/pkg/graceful"
)

var (
	cmdRelayer = &cobra.Command{
		Use:   "relayer",
		Short: "Relayer commands",
	}

	cmdRelayerRun = &cobra.Command{
		Use:   "run",
		Short: "Run the relayer",
		RunE:  relayerRun,
	}

	cmdRelayerRelay = &cobra.Command{
		Use:   "relay",
		Short: "Trigger relaying of the packets emitted by a source transaction",
		RunE:  relayerRelay,
	}

	cmdRelayerStatus = &cobra.Command{
		Use:   useStatus,
		Short: "Query per-packet relay status for a source transaction",
		RunE:  relayerStatus,
	}
)

var (
	relayerOptions           RelayerOptions
	flagRelayerNoMigrate     bool
	flagRelayerHost          string
	flagRelayerTxHash        string
	flagRelayerSourceChainID string
)

func relayerRun(cmd *cobra.Command, _ []string) error {
	cfg, err := setupHomeWithConfig()
	if err != nil {
		return err
	}

	app, err := bootstrap.BuildRelayer(cfg, bootstrap.RelayerOptions{
		ProverFactories: relayerOptions.ProverFactories,
	})
	if err != nil {
		return err
	}

	if flagRelayerNoMigrate {
		app.Logger.Info("--no-migrate flag passed, skipping migrations")
	} else {
		applied, migrateErr := app.Store.MigrateUp()
		switch {
		case migrateErr != nil:
			return errors.Wrap(migrateErr, "failed to migrate database")
		case applied == 0:
			app.Logger.Info("No migrations to apply")
		case applied > 0:
			app.Logger.Info("Migrated database", "migrations_applied", applied)
		}
	}

	app.Logger.Info("Starting relayer")

	address, err := app.Server.Start()
	if err != nil {
		app.Logger.Error("Failed to start relayer server", "err", err)
		return err
	}

	if err := app.RelayerService.Start(); err != nil {
		app.Logger.Error("Failed to start relayer dispatch loop", "err", err)
		_ = app.Server.Stop()
		return err
	}

	connected := make([]string, 0, len(cfg.Chains))
	for _, chain := range cfg.Chains {
		connected = append(connected, chain.ChainID)
	}
	if err := json.NewEncoder(cmd.OutOrStdout()).Encode(relayerv2.ProcessReadiness{
		Event:           relayerv2.ProcessReadinessEvent,
		ChainsConnected: connected,
		HTTP:            address.String(),
	}); err != nil {
		_ = app.RelayerService.Stop()
		_ = app.Server.Stop()
		return err
	}

	// executes from last to first
	graceful.AddCallback(app.Store.Close)
	graceful.AddCallback(app.Server.Stop)
	graceful.AddCallback(app.RelayerService.Stop)

	// blocking
	return graceful.WaitShutdown()
}

func relayerRelay(cmd *cobra.Command, _ []string) error {
	return relayerCall(cmd, relayerv2.RelayerApiServiceClient.Relay, &relayerv2.RelayRequest{
		TxHash:        flagRelayerTxHash,
		SourceChainId: flagRelayerSourceChainID,
		Selection:     &relayerv2.RelayRequest_AllPackets{AllPackets: &relayerv2.AllPackets{}},
	})
}

func relayerStatus(cmd *cobra.Command, _ []string) error {
	return relayerCall(cmd, relayerv2.RelayerApiServiceClient.Status, &relayerv2.StatusRequest{
		TxHash: flagRelayerTxHash, SourceChainId: flagRelayerSourceChainID,
	})
}

// relayerCall resolves this config's relayer address, sends req via call,
// and prints the response as JSON.
func relayerCall[Req, Resp any](
	cmd *cobra.Command,
	call func(relayerv2.RelayerApiServiceClient, context.Context, *connect.Request[Req]) (*connect.Response[Resp], error),
	req *Req,
) error {
	cfg, err := setupHomeWithConfig()
	if err != nil {
		return err
	}

	address := flagRelayerHost
	if address == "" {
		if cfg.Server.ListenAddress == "" {
			return errors.New("server.listenAddr is not configured; pass --host to target a server directly")
		}
		address = cfg.Server.ListenAddress
	}

	client := relayerv2.NewRelayerApiServiceClient(
		newGRPCHTTPClient(), "http://"+dialableAddress(address), connect.WithGRPC(),
	)

	res, err := call(client, cmd.Context(), connect.NewRequest(req))
	if err != nil {
		return errors.Wrap(err, cmd.Name())
	}

	if pm, ok := any(res.Msg).(proto.Message); ok {
		return config.PrintProtoJSON(pm)
	}

	return config.PrintJSON(res.Msg)
}
