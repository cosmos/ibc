// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"sort"
	"strings"

	"connectrpc.com/connect"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

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
var packetStates = map[string]relayerv2.PacketState{
	"not-selected": relayerv2.PacketState_PACKET_STATE_NOT_SELECTED,
	"pending":      relayerv2.PacketState_PACKET_STATE_PENDING,
	"succeeded":    relayerv2.PacketState_PACKET_STATE_SUCCEEDED,
	"timed-out":    relayerv2.PacketState_PACKET_STATE_TIMED_OUT,
	"rejected":     relayerv2.PacketState_PACKET_STATE_REJECTED,
	"relay-failed": relayerv2.PacketState_PACKET_STATE_RELAY_FAILED,
}

func packetStateNames() []string {
	names := make([]string, 0, len(packetStates))
	for name := range packetStates {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}

func relayerRun(cmd *cobra.Command, _ []string) error {
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
	client, err := relayerClient()
	if err != nil {
		return err
	}

	res, err := client.Relay(cmd.Context(), connect.NewRequest(&relayerv2.RelayRequest{
		TxHash:        flagRelayerTxHash,
		SourceChainId: flagRelayerSourceChainID,
		Selection:     &relayerv2.RelayRequest_AllPackets{AllPackets: &relayerv2.AllPackets{}},
	}))
	if err != nil {
		return errors.Wrap(err, cmd.Name())
	}

	return config.PrintProtoJSON(res.Msg)
}

func relayerPackets(cmd *cobra.Command, _ []string) error {
	filter := &relayerv2.PacketFilter{}

	optional := func(value string, into **string) {
		if value != "" {
			v := value
			*into = &v
		}
	}

	optional(flagRelayerSourceChainID, &filter.SourceChainId)
	optional(flagRelayerPacketsDestChainID, &filter.DestinationChainId)
	optional(flagRelayerPacketsSrcClientID, &filter.SourceClientId)
	optional(flagRelayerPacketsDestClientID, &filter.DestinationClientId)
	optional(flagRelayerTxHash, &filter.SourceTxHash)

	if cmd.Flags().Changed("sequence") {
		filter.SequenceNumber = &flagRelayerPacketsSequence
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

	req := &relayerv2.PacketsRequest{
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

	return config.PrintProtoJSON(res.Msg)
}

// relayerPacketsAll follows next_cursor to completion and prints all packets as
// one response
func relayerPacketsAll(cmd *cobra.Command, req *relayerv2.PacketsRequest) error {
	client, err := relayerClient()
	if err != nil {
		return err
	}

	all := &relayerv2.PacketsResponse{}

	for {
		res, err := client.Packets(cmd.Context(), connect.NewRequest(req))
		if err != nil {
			return errors.Wrap(err, cmd.Name())
		}

		all.Packets = append(all.Packets, res.Msg.GetPackets()...)

		if !res.Msg.GetHasMore() {
			return config.PrintProtoJSON(all)
		}

		req.Cursor = res.Msg.GetNextCursor()
	}
}

// relayerClient dials the relayer resolved from this config, or --host.
func relayerClient() (relayerv2.RelayerApiServiceClient, error) {
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

	return relayerv2.NewRelayerApiServiceClient(
		newGRPCHTTPClient(), "http://"+dialableAddress(address), connect.WithGRPC(),
	), nil
}
