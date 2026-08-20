// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"

	"connectrpc.com/connect"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/proto"

	"github.com/cosmos/ibc/link/api/v2/relayer"
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
	flagRelayerNoMigrate     bool
	flagRelayerHost          string
	flagRelayerTxHash        string
	flagRelayerSourceChainID string
)

func relayerRun(_ *cobra.Command, _ []string) error {
	cfg, err := setupHomeWithConfig()
	if err != nil {
		return err
	}

	app, err := bootstrap.BuildRelayer(cfg)
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

	app.Logger.Info("Readiness", "readiness", relayer.ProcessReadiness{
		Event:           relayer.ProcessReadinessEvent,
		ChainsConnected: connected,
		HTTP:            address.String(),
	})

	// executes from last to first
	graceful.AddCallback(app.Store.Close)
	graceful.AddCallback(app.Server.Stop)
	graceful.AddCallback(app.RelayerService.Stop)

	// blocking
	return graceful.WaitShutdown()
}

func relayerRelay(cmd *cobra.Command, _ []string) error {
	return relayerCall(cmd, relayer.RelayerApiServiceClient.Relay, &relayer.RelayRequest{
		TxHash:        flagRelayerTxHash,
		SourceChainId: flagRelayerSourceChainID,
		Selection:     &relayer.RelayRequest_AllPackets{AllPackets: &relayer.AllPackets{}},
	})
}

func relayerStatus(cmd *cobra.Command, _ []string) error {
	return relayerCall(cmd, relayer.RelayerApiServiceClient.Status, &relayer.StatusRequest{
		TxHash: flagRelayerTxHash, SourceChainId: flagRelayerSourceChainID,
	})
}

// relayerCall resolves this config's relayer address, sends req via call,
// and prints the response as JSON.
func relayerCall[Req, Resp any](
	cmd *cobra.Command,
	call func(relayer.RelayerApiServiceClient, context.Context, *connect.Request[Req]) (*connect.Response[Resp], error),
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

	client := relayer.NewRelayerApiServiceClient(
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
