// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/ibclink"
	relayerv2 "github.com/cosmos/ibc/link/api/v2/relayer"
)

type remoteProverEnv struct {
	env        *environment.Environment
	driver     *ibclink.Driver
	deployment *e2etest.Deployment
	route      e2etest.Route
	sender     e2etest.Signer
}

func startRemoteProverEnv(t *testing.T, requirements e2etest.EVMRequirements) remoteProverEnv {
	t.Helper()

	spec, runtime := attestedMesh(e2etest.EVMChains(t, requirements, e2etest.ChainA, e2etest.ChainB))
	env := e2etest.Start(t, spec, runtime)
	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSigner(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)

	prover := e2etest.StartProver(t, env, relayerSigner)

	driver, deployment := e2etest.DeployWithRelayerConfig(t, env, sender, relayerSigner,
		func(cfg *ibclink.RelayerConfig) {
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

	transferApp := e2etest.NewTransfer(t, setup.env, setup.deployment, setup.sender, setup.route)
	relayer := e2etest.StartRelayer(t, setup.driver, setup.env)
	ctx := t.Context()

	amount := new(big.Int).Mul(big.NewInt(500_000), big.NewInt(1_000_000_000_000_000_000))
	transfer, err := transferApp.Send(ctx, e2etest.TransferRequest{Amount: amount})
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyEscrowed(ctx))

	status, err := e2etest.AwaitState(ctx, relayer, transfer.PacketTx(),
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

	transferApp := e2etest.NewTransfer(t, setup.env, setup.deployment, setup.sender, setup.route)
	relayer := e2etest.StartRelayer(t, setup.driver, setup.env)
	ctx := t.Context()

	require.NoError(t, relayer.Stop(ctx))

	transfer, err := transferApp.Send(ctx, e2etest.TransferRequest{
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

	status, err := e2etest.AwaitState(ctx, relayer, transfer.PacketTx(),
		relayerv2.PacketState_PACKET_STATE_TIMED_OUT)
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyRefunded(ctx, status.GetTimeoutTx().GetTxHash()))
	require.NoError(t, transfer.VerifyNotMinted(ctx))
	require.NoError(t, transfer.VerifyCommitmentCleared(ctx))
}

func TestRemoteProver_ErrorAcknowledgement(t *testing.T) {
	t.Parallel()

	setup := startRemoteProverEnv(t, e2etest.EVMRequirements{})

	gmp := e2etest.NewGMP(t, setup.env, setup.deployment, setup.sender, setup.route)
	relayer := e2etest.StartRelayer(t, setup.driver, setup.env)
	ctx := t.Context()

	call, err := gmp.Call(ctx, e2etest.GMPRequest{Payload: invalidGMPPayload})
	require.NoError(t, err)

	_, err = e2etest.AwaitState(ctx, relayer, call.PacketTx(),
		relayerv2.PacketState_PACKET_STATE_REJECTED)
	require.NoError(t, err)
	require.NoError(t, call.VerifyCounterUnchanged(ctx))
}
