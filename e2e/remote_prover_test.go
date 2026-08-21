// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	"context"
	"errors"
	"math/big"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/ibclink"
	relayerv2 "github.com/cosmos/ibc/link/api/v2/relayer"
	"github.com/cosmos/ibc/link/testutil/proverservice"
)

// startProverService runs a ProverService built from the relayer's config. It
// is separate: the relayer holds only the endpoint.
func startProverService(t *testing.T, driver *ibclink.Driver, address string) {
	t.Helper()

	// The config expands chain RPCs from the environment; so must this.
	vars, release, err := driver.ChainRPCEnv()
	require.NoError(t, err, "resolve chain rpc env")

	t.Cleanup(release)

	for name, value := range vars {
		t.Setenv(name, value)
	}

	server, err := proverservice.NewAttestationServer(t.Context(), driver.ConfigPath())
	require.NoError(t, err, "build prover service")

	listener, err := net.Listen("tcp", address)
	require.NoError(t, err, "listen for prover service")

	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)

		if err := <-served; err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("prover service: %v", err)
		}
	})
}

// reserveLoopbackAddress picks a port, since the config names it before the
// service starts.
func reserveLoopbackAddress(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	address := listener.Addr().String()
	require.NoError(t, listener.Close())

	return address
}

// Relays a real packet with every proof fetched over gRPC from a service the
// relayer does not host.
// Not parallel: t.Setenv is needed for the in-process prover.
func TestRemoteProver_RelaysPacket(t *testing.T) {
	spec, runtime := attestedMesh(e2etest.EVMChains(t, e2etest.EVMRequirements{}, e2etest.ChainA, e2etest.ChainB))
	env := e2etest.Start(t, spec, runtime)
	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSigner(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)

	proverAddress := reserveLoopbackAddress(t)

	driver, deployment := e2etest.DeployWithRelayerConfig(t, env, sender, relayerSigner,
		func(cfg *ibclink.RelayerConfig) {
			for i := range cfg.Connections {
				cfg.Connections[i].ProverURL = "http://" + proverAddress
			}
		}, route)

	// The service must answer before the relayer asks it for a proof.
	startProverService(t, driver, proverAddress)

	transferApp := e2etest.NewTransfer(t, env, deployment, sender, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	amount := new(big.Int).Mul(big.NewInt(500_000), big.NewInt(1_000_000_000_000_000_000))
	transfer, err := transferApp.Send(ctx, e2etest.TransferRequest{
		Amount: amount,
		Memo:   "remote-prover",
	})
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyEscrowed(ctx))

	status, err := e2etest.AwaitState(ctx, relayer, transfer.PacketTx(),
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)

	// Every proof came from the remote service, so a dropped field fails here.
	require.NoError(t, transfer.VerifyDelivered(ctx))
	require.NoError(t, transfer.VerifyCommitmentCreated(ctx))
	require.NoError(t, transfer.VerifyReceiptCreated(ctx))
	require.NoError(t, transfer.VerifyCommitmentCleared(ctx))
	require.NoError(t, transfer.VerifyAcknowledgementWritten(ctx, status.GetRecvTx().GetTxHash()))
	require.NoError(t, transfer.VerifyAcknowledgementExecuted(ctx, status.GetAckTx().GetTxHash()))
}
