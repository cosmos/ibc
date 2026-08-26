// SPDX-License-Identifier: Apache-2.0

package main

import (
	"maps"
	"slices"
	"strings"

	"connectrpc.com/connect"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

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

	cmdRelayerPackets = &cobra.Command{
		Use:   "packets",
		Short: "List the packets the relayer is aware of and their relay status",
		Long: "List the packets the relayer is aware of, most recent first.\n\n" +
			"With no filters every known packet is listed, bounded by --limit. " +
			"Filters combine: a packet must match all of them.",
		Example: "  ibc relayer packets --state pending\n" +
			"  ibc relayer packets --chain-id 1 --tx-hash 0xabc\n" +
			"  ibc relayer packets --source-client-id base-0 --limit 20",
		RunE: relayerPackets,
	}
)

var (
	flagRelayerNoMigrate     bool
	flagRelayerHost          string
	flagRelayerTxHash        string
	flagRelayerSourceChainID string

	flagRelayerPacketsDestChainID  string
	flagRelayerPacketsSrcClientID  string
	flagRelayerPacketsDestClientID string
	flagRelayerPacketsState        string
	flagRelayerPacketsSequence     uint64
	flagRelayerPacketsLimit        uint32
	flagRelayerPacketsCursor       string
	flagRelayerPacketsAll          bool
)

// packetStates maps the --state flag to its wire value.
var packetStates = map[string]relayer.PacketState{
	"not-selected": relayer.PacketState_PACKET_STATE_NOT_SELECTED,
	"pending":      relayer.PacketState_PACKET_STATE_PENDING,
	"succeeded":    relayer.PacketState_PACKET_STATE_SUCCEEDED,
	"timed-out":    relayer.PacketState_PACKET_STATE_TIMED_OUT,
	"rejected":     relayer.PacketState_PACKET_STATE_REJECTED,
	"relay-failed": relayer.PacketState_PACKET_STATE_RELAY_FAILED,
}

func packetStateNames() []string {
	return slices.Sorted(maps.Keys(packetStates))
}

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
		app.Logger.Error("Failed to start relayer loop", "err", err)
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
	client, err := relayerClient()
	if err != nil {
		return err
	}

	res, err := client.Relay(cmd.Context(), connect.NewRequest(&relayer.RelayRequest{
		TxHash:        flagRelayerTxHash,
		SourceChainId: flagRelayerSourceChainID,
		Selection:     &relayer.RelayRequest_AllPackets{AllPackets: &relayer.AllPackets{}},
	}))
	if err != nil {
		return errors.Wrap(err, cmd.Name())
	}

	return config.PrintJSON(res.Msg)
}

func relayerPackets(cmd *cobra.Command, _ []string) error {
	filter := &relayer.PacketFilter{
		SourceChainId:       optional(flagRelayerSourceChainID),
		DestinationChainId:  optional(flagRelayerPacketsDestChainID),
		SourceClientId:      optional(flagRelayerPacketsSrcClientID),
		DestinationClientId: optional(flagRelayerPacketsDestClientID),
		SourceTxHash:        optional(flagRelayerTxHash),
		SequenceNumber:      optional(flagRelayerPacketsSequence),
	}

	if flagRelayerPacketsState != "" {
		state, ok := packetStates[strings.ToLower(flagRelayerPacketsState)]
		if !ok {
			return errors.Errorf(
				"invalid --state %q, expected one of: %s",
				flagRelayerPacketsState, strings.Join(packetStateNames(), ", "),
			)
		}

		filter.State = &state
	}

	req := &relayer.PacketsRequest{
		Filter: filter,
		Limit:  flagRelayerPacketsLimit,
		Cursor: flagRelayerPacketsCursor,
	}

	if flagRelayerPacketsAll {
		return relayerPacketsAll(cmd, req)
	}

	client, err := relayerClient()
	if err != nil {
		return err
	}

	res, err := client.Packets(cmd.Context(), connect.NewRequest(req))
	if err != nil {
		return errors.Wrap(err, cmd.Name())
	}

	return config.PrintJSON(res.Msg)
}

// relayerPacketsAll follows next_cursor to completion and prints all packets as
// one response
func relayerPacketsAll(cmd *cobra.Command, req *relayer.PacketsRequest) error {
	client, err := relayerClient()
	if err != nil {
		return err
	}

	all := &relayer.PacketsResponse{}

	for {
		res, err := client.Packets(cmd.Context(), connect.NewRequest(req))
		if err != nil {
			return errors.Wrap(err, cmd.Name())
		}

		all.Packets = append(all.Packets, res.Msg.GetPackets()...)

		if !res.Msg.GetHasMore() {
			return config.PrintJSON(all)
		}

		req.Cursor = res.Msg.GetNextCursor()
	}
}

// relayerClient dials the relayer resolved from this config, or --host.
func relayerClient() (relayer.RelayerApiServiceClient, error) {
	cfg, err := setupHomeWithConfig()
	if err != nil {
		return nil, err
	}

	address := flagRelayerHost
	if address == "" {
		if cfg.Server.ListenAddress == "" {
			return nil, errors.New("server.listenAddr is not configured; pass --host to target a server directly")
		}
		address = cfg.Server.ListenAddress
	}

	return relayer.NewRelayerApiServiceClient(
		newGRPCHTTPClient(), "http://"+dialableAddress(address), connect.WithGRPC(),
	), nil
}

func optional[T comparable](value T) *T {
	var zero T
	if value == zero {
		return nil
	}

	return &value
}
