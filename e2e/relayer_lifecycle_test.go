// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	relayerv2 "github.com/cosmos/ibc/cli/api/v2/relayer"
	"github.com/cosmos/ibc/e2e/internal/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/ibccli"
)

func TestAutoRelay_PacketIsClearedOnRestart(t *testing.T) {
	t.Parallel()
	t.Skip("TODO")

	// todo deploy stuff
	// todo send packet1 -> OK
	// todo disable relayer
	// todo send packet2, on chain
	// todo start relayer with clearOnStart=true
	// todo packet2 should be delivered eventually
}

func TestAutoRelay_AllPacketsAreClearedOnStart(t *testing.T) {
	t.Parallel()

	// ARRANGE
	// Given attested setup
	spec, runtime := attestedMesh(e2etest.EVMChains(t, e2etest.EVMRequirements{}, e2etest.ChainA, e2etest.ChainB))
	env := e2etest.Start(t, spec, runtime)

	// Given signers
	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSigner(t)

	// Given relayer deployment with clearing enabled and two routes
	routeAB := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	routeBA := e2etest.BtoA(e2etest.ChainB, e2etest.ChainA)

	configMutator := func(cfg *ibccli.RelayerConfig) {
		cfg.ClearOnStart = true
		cfg.ClearInterval = 2 * time.Second
	}

	driver, deployment := e2etest.DeployWithRelayerConfig(
		t, env, sender, relayerSigner, configMutator, routeAB, routeBA,
	)

	// Given deployed transfer apps for both routes
	transferAppAB := e2etest.NewTransfer(t, env, deployment, sender, routeAB)
	transferAppBA := e2etest.NewTransfer(t, env, deployment, sender, routeBA)

	ctx := t.Context()

	// Given 4 packets send BEFORE relayer was started
	send := func(app *e2etest.Transfer, amount *big.Int) *e2etest.TransferSend {
		transfer, err := app.Send(ctx, e2etest.TransferRequest{Amount: amount})
		require.NoError(t, err)
		require.NoError(t, transfer.VerifyEscrowed(ctx))

		return transfer
	}

	t1 := send(transferAppAB, big.NewInt(100_000))
	t2 := send(transferAppAB, big.NewInt(200_000))
	t3 := send(transferAppBA, big.NewInt(300_000))
	t4 := send(transferAppBA, big.NewInt(400_000))

	// ACT start the relayer
	// discover and relay all 4 packets without any websocket event.
	relayer := e2etest.StartRelayer(t, driver, env)

	// ASSERT
	// Every packet reaches SUCCEEDED and the destination balances reflect delivery.
	assert := func(transfer *e2etest.TransferSend) {
		_, err := e2etest.AwaitState(ctx, relayer, transfer.PacketTx(), relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
		require.NoError(t, err)
		require.NoError(t, transfer.VerifyDelivered(ctx))
	}

	assert(t1)
	assert(t2)
	assert(t3)
	assert(t4)
}

func TestAutoRelay_SubscriptionReconnect(t *testing.T) {
	t.Parallel()
	t.Skip("TODO")

	// todo deploy stuff
	// todo send packet1 -> OK
	// todo somehow cut off websocket connection
	// todo wait for reconnect
	// todo send packet2 -> OK
}

func TestManualRelay_RequestSurvivesRestart(t *testing.T) {
	t.Parallel()
	spec, runtime := attestedMesh(e2etest.EVMChains(t,
		e2etest.EVMRequirements{ControlledMining: true}, e2etest.ChainA, e2etest.ChainB))
	env := e2etest.Start(t, spec, runtime)
	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSigner(t)
	route := e2etest.ManualAtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.Deploy(t, env, sender, relayerSigner, route)
	transferApp := e2etest.NewTransfer(t, env, deployment, sender, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	chainB, err := env.Chain(e2etest.ChainB)
	require.NoError(t, err)
	mining, err := chainB.Mining()
	require.NoError(t, err)

	// Keep destination mining paused across restart so delivery cannot finish before the new Relayer is up.
	var transfer *e2etest.TransferSend
	require.NoError(t, mining.WithPaused(ctx, func() error {
		transfer, err = transferApp.Send(ctx, e2etest.TransferRequest{Amount: big.NewInt(888_000)})
		require.NoError(t, err)
		require.NoError(t, transfer.VerifyEscrowed(ctx))

		require.NoError(t, e2etest.RelayAll(ctx, relayer, transfer.PacketTx()))
		require.NoError(t, relayer.Stop(ctx))
		relayer = e2etest.StartRelayer(t, driver, env)

		// The restarted relayer still tracks the packet from its store.
		require.NoError(t, e2etest.AwaitStable(ctx, relayer, transfer.PacketTx(),
			relayerv2.PacketState_PACKET_STATE_PENDING))
		require.NoError(t, transfer.VerifyNotMinted(ctx))
		return nil
	}))

	_, err = e2etest.AwaitState(ctx, relayer, transfer.PacketTx(),
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyDelivered(ctx))
}
