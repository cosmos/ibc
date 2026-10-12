// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	relayerv2 "github.com/cosmos/ibc/cli/api/v2/relayer"
	"github.com/cosmos/ibc/e2e/internal/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/ibccli"
	"github.com/cosmos/ibc/e2e/internal/harness/traffic"
)

type remoteProverEnv struct {
	env        *environment.Environment
	driver     *ibccli.Driver
	deployment *traffic.Deployment
	route      traffic.Route
	sender     traffic.Signer
}

func startRemoteProverEnv(t *testing.T, requirements e2etest.EVMRequirements) remoteProverEnv {
	t.Helper()

	spec, runtime := traffic.AttestedMesh(e2etest.EVMChains(t, requirements, traffic.ChainA, traffic.ChainB)...)
	env := e2etest.Start(t, spec, runtime)
	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSigner(t)
	route := traffic.NewRoute(traffic.ChainA, traffic.ChainB)

	prover := e2etest.StartProver(t, env, relayerSigner)

	driver, deployment := e2etest.DeployWithRelayerConfig(t, env, sender, relayerSigner,
		func(cfg *ibccli.RelayerConfig) {
			for i := range cfg.Connections {
				cfg.Connections[i].ProverURL = "http://" + prover.Address()
			}
		}, route)

	return remoteProverEnv{env: env, driver: driver, deployment: deployment, route: route, sender: sender}
}

// Relays a real packet with every proof fetched over gRPC from a service the
// relayer does not host.
func TestRemoteProver_RelaysPacket(t *testing.T) {
	t.Parallel()

	setup := startRemoteProverEnv(t, e2etest.EVMRequirements{})

	transferApp, err := traffic.NewTransfer(setup.env, setup.deployment, setup.sender, setup.route)
	require.NoError(t, err)
	relayer := e2etest.StartRelayer(t, setup.driver, setup.env)
	ctx := t.Context()

	amount := new(big.Int).Mul(big.NewInt(500_000), big.NewInt(1_000_000_000_000_000_000))
	transfer, err := transferApp.Send(ctx, traffic.TransferRequest{Amount: amount})
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyEscrowed(ctx))

	status, err := traffic.AwaitState(ctx, relayer, transfer.PacketTx(),
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)

	require.NoError(t, transfer.VerifyDelivered(ctx))
	require.NoError(t, transfer.VerifyCommitmentCreated(ctx))
	require.NoError(t, transfer.VerifyReceiptCreated(ctx))
	require.NoError(t, transfer.VerifyCommitmentCleared(ctx))
	require.NoError(t, transfer.VerifyAcknowledgementWritten(ctx, status.GetRecvTx().GetTxHash()))
	require.NoError(t, transfer.VerifyAcknowledgementExecuted(ctx, status.GetAckTx().GetTxHash()))
}

func TestRemoteProver_TimesOutPacket(t *testing.T) {
	t.Parallel()

	setup := startRemoteProverEnv(t, e2etest.EVMRequirements{ControlledMining: true})

	transferApp, err := traffic.NewTransfer(setup.env, setup.deployment, setup.sender, setup.route)
	require.NoError(t, err)
	relayer := e2etest.StartRelayer(t, setup.driver, setup.env)
	ctx := t.Context()

	require.NoError(t, relayer.Stop(ctx))

	transfer, err := transferApp.Send(ctx, traffic.TransferRequest{
		Amount:  big.NewInt(3_000_000),
		Timeout: packetTimeout,
	})
	require.NoError(t, err)

	chainB, err := setup.env.Chain(setup.route.Destination)
	require.NoError(t, err)

	mining, err := chainB.Mining()
	require.NoError(t, err)
	require.NoError(t, mining.AdvanceTime(ctx, packetTimeoutAdvance))

	relayer = e2etest.StartRelayer(t, setup.driver, setup.env)

	status, err := traffic.AwaitState(ctx, relayer, transfer.PacketTx(),
		relayerv2.PacketState_PACKET_STATE_TIMED_OUT)
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyRefunded(ctx, status.GetTimeoutTx().GetTxHash()))
	require.NoError(t, transfer.VerifyNotMinted(ctx))
	require.NoError(t, transfer.VerifyCommitmentCleared(ctx))
}

func TestRemoteProver_ErrorAcknowledgement(t *testing.T) {
	t.Parallel()

	setup := startRemoteProverEnv(t, e2etest.EVMRequirements{})

	gmp, err := traffic.NewGMP(setup.env, setup.deployment, setup.sender, setup.route)
	require.NoError(t, err)
	relayer := e2etest.StartRelayer(t, setup.driver, setup.env)
	ctx := t.Context()

	call, err := gmp.Call(ctx, traffic.GMPRequest{Payload: invalidGMPPayload})
	require.NoError(t, err)

	_, err = traffic.AwaitState(ctx, relayer, call.PacketTx(),
		relayerv2.PacketState_PACKET_STATE_REJECTED)
	require.NoError(t, err)
	require.NoError(t, call.VerifyCounterUnchanged(ctx))
}
