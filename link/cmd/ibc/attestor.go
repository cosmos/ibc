// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"

	"connectrpc.com/connect"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	attestorv2 "github.com/cosmos/ibc/link/api/v2/attestor"
	"github.com/cosmos/ibc/link/config"
	"github.com/cosmos/ibc/link/internal/bootstrap"
	"github.com/cosmos/ibc/link/internal/pkg/graceful"
)

var (
	flagAttestorHost   string
	flagAttestorHeight uint64
)

var (
	cmdAttestor = &cobra.Command{
		Use:   "attestor",
		Short: "Attestor commands",
	}

	cmdAttestorRun = &cobra.Command{
		Use:   "run",
		Short: "Run the attestor",
		RunE:  attestorRun,
	}

	cmdAttestorInfo = &cobra.Command{
		Use:   "info [name]",
		Short: "Query a local attestor's identity",
		Args:  cobra.ExactArgs(1),
		RunE:  attestorInfo,
	}

	cmdAttestorLatestHeight = &cobra.Command{
		Use:   "latest-height [name]",
		Short: "Query a local attestor's latest attestable height",
		Args:  cobra.ExactArgs(1),
		RunE:  attestorLatestHeight,
	}

	cmdAttestorStateAttestation = &cobra.Command{
		Use:   "state-attestation [name]",
		Short: "Query a local attestor for a state attestation at --height",
		Args:  cobra.ExactArgs(1),
		RunE:  attestorStateAttestation,
	}
)

func attestorRun(cmd *cobra.Command, _ []string) error {
	cfg, err := setupHomeWithConfig()
	if err != nil {
		return err
	}

	app, err := bootstrap.BuildAttestor(cfg)
	if err != nil {
		return err
	}

	app.Logger.Info("Starting attestor")

	address, err := app.Server.Start()
	if err != nil {
		app.Logger.Error("Failed to start attestor server", "err", err)
		return err
	}
	if err := json.NewEncoder(cmd.OutOrStdout()).Encode(attestorv2.ProcessReadiness{
		Event: attestorv2.ProcessReadinessEvent,
		HTTP:  address.String(),
	}); err != nil {
		_ = app.Server.Stop()
		return err
	}

	graceful.AddCallback(app.Server.Stop)

	// blocking
	return graceful.WaitShutdown()
}

func attestorInfo(cmd *cobra.Command, args []string) error {
	name := args[0]
	return attestorCall(cmd, name, attestorv2.AttestationServiceClient.Info, &attestorv2.InfoRequest{Attestor: name})
}

func attestorLatestHeight(cmd *cobra.Command, args []string) error {
	name := args[0]
	return attestorCall(
		cmd, name, attestorv2.AttestationServiceClient.LatestHeight, &attestorv2.LatestHeightRequest{Attestor: name},
	)
}

func attestorStateAttestation(cmd *cobra.Command, args []string) error {
	name := args[0]
	return attestorCall(
		cmd,
		name,
		attestorv2.AttestationServiceClient.StateAttestation,
		&attestorv2.StateAttestationRequest{
			Attestor: name, Height: flagAttestorHeight,
		},
	)
}

// attestorCall resolves name's dial address, sends req via call, and prints
// the response as JSON.
func attestorCall[Req, Resp any](
	cmd *cobra.Command,
	name string,
	call func(attestorv2.AttestationServiceClient, context.Context, *connect.Request[Req]) (*connect.Response[Resp], error),
	req *Req,
) error {
	cfg, err := setupHomeWithConfig()
	if err != nil {
		return err
	}

	address := flagAttestorHost
	if address == "" {
		if err = requireLocalAttestor(cfg, name); err != nil {
			return err
		}
		if cfg.Server.ListenAddress == "" {
			return errors.New("server.listenAddr is not configured; pass --host to target a server directly")
		}
		address = cfg.Server.ListenAddress
	}

	client := attestorv2.NewAttestationServiceClient(
		newGRPCHTTPClient(), "http://"+dialableAddress(address), connect.WithGRPC(),
	)

	res, err := call(client, cmd.Context(), connect.NewRequest(req))
	if err != nil {
		return errors.Wrap(err, cmd.Name())
	}

	return printJSON(res.Msg)
}

// requireLocalAttestor errors unless name is a locally-run attestor in cfg --
// a remote attestor's process isn't this config's server, so dialing it here
// would be wrong.
func requireLocalAttestor(cfg config.Config, name string) error {
	attestor, ok := cfg.AttestorByName(name)
	if !ok {
		return errors.Errorf("attestor %q not found in config", name)
	}
	if attestor.Type != config.AttestorTypeLocal {
		return errors.Errorf("attestor %q is not local: pass --host to target it directly", name)
	}

	return nil
}

// dialableAddress rewrites an unspecified listen host (e.g. "0.0.0.0", the
// config default) to "127.0.0.1" -- some platforms refuse outbound
// connections to 0.0.0.0 even though it's a valid listen address.
func dialableAddress(address string) string {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return address
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
		host = "127.0.0.1"
	}

	return net.JoinHostPort(host, port)
}

// https://connectrpc.com/docs/go/getting-started/#make-requests
func newGRPCHTTPClient() *http.Client {
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetHTTP2(true)
	protocols.SetUnencryptedHTTP2(true)

	return &http.Client{
		Transport: &http.Transport{Protocols: protocols},
	}
}
