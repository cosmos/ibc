// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	"math/big"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	relayerv2 "github.com/cosmos/ibc/cli/api/v2/relayer"
	"github.com/cosmos/ibc/e2e/internal/e2etest"
	chainevm "github.com/cosmos/ibc/e2e/internal/harness/chain/evm"
	"github.com/cosmos/ibc/e2e/internal/harness/ibccli"
)

func TestAutoRelay_PacketIsClearedOnRestart(t *testing.T) {
	t.Parallel()

	// ARRANGE
	// Given attested setup
	spec, runtime := attestedMesh(e2etest.EVMChains(t, e2etest.EVMRequirements{}, e2etest.ChainA, e2etest.ChainB))
	env := e2etest.Start(t, spec, runtime)

	// Given signers
	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSigner(t)

	// Given relayer deployment with clearing enabled
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	configMutator := func(cfg *ibccli.RelayerConfig) {
		cfg.ClearOnStart = true
		cfg.ClearInterval = 2 * time.Second
	}

	driver, deployment := e2etest.DeployWithRelayerConfig(t, env, sender, relayerSigner, configMutator, route)
	transferApp := e2etest.NewTransfer(t, env, deployment, sender, route)
	ctx := t.Context()

	// Given started relayer
	relayer := e2etest.StartRelayer(t, driver, env)

	// Given sample transfer sent and auto-relayed
	transfer1 := mustSend(t, transferApp, big.NewInt(100_000))
	mustDeliver(t, relayer, transfer1)

	// ACT #1
	require.NoError(t, relayer.Stop(ctx))

	// ACT #2
	// Send packet2 while the relayer is down so the packet is not auto-relayed.
	transfer2 := mustSend(t, transferApp, big.NewInt(200_000))

	// ACT #3
	relayer = e2etest.StartRelayer(t, driver, env)

	// ASSERT
	mustDeliver(t, relayer, transfer2)
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

	// Given 4 packets send BEFORE relayer was started
	t1 := mustSend(t, transferAppAB, big.NewInt(100_000))
	t2 := mustSend(t, transferAppAB, big.NewInt(200_000))
	t3 := mustSend(t, transferAppBA, big.NewInt(300_000))
	t4 := mustSend(t, transferAppBA, big.NewInt(400_000))

	// ACT start the relayer
	// discover and relay all 4 packets without any websocket event.
	relayer := e2etest.StartRelayer(t, driver, env)

	// ASSERT
	// Every packet reaches SUCCEEDED and the destination balances reflect delivery.
	mustDeliver(t, relayer, t1)
	mustDeliver(t, relayer, t2)
	mustDeliver(t, relayer, t3)
	mustDeliver(t, relayer, t4)
}

func TestAutoRelay_SubscriptionReconnect(t *testing.T) {
	t.Parallel()

	// ARRANGE
	// Given attested setup
	spec, runtime := attestedMesh(e2etest.EVMChains(t, e2etest.EVMRequirements{}, e2etest.ChainA, e2etest.ChainB))
	env := e2etest.Start(t, spec, runtime)

	// Given signers
	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSigner(t)

	// Given relayer deployment with auto-relay
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)

	// Given chain A
	chainA, err := env.Chain(e2etest.ChainA)
	require.NoError(t, err)
	chainAID := strconv.FormatUint(chainA.EVMChainID(), 10)

	// Given (!) websocket proxy for chain A that allows to cut off the relayer's websocket subscription.
	// relayer <-> ws_proxy <-> chain A
	chainWSProxy := chainevm.NewWebSocketProxy(t, chainA.WSURL())

	// Given NO clearOnStart and long clearInterval
	// (so the clearing pass is not triggered)
	configMutator := func(cfg *ibccli.RelayerConfig) {
		cfg.ClearOnStart = false
		cfg.ClearInterval = 1 * time.Hour

		for i := range cfg.Chains {
			if cfg.Chains[i].ChainID == chainAID {
				cfg.Chains[i].WS = chainWSProxy.URL()
				return
			}
		}

		t.Fatalf("chain %s not found in relayer config", chainAID)
	}

	driver, deployment := e2etest.DeployWithRelayerConfig(t, env, sender, relayerSigner, configMutator, route)

	// Given deployed transfer app
	transferApp := e2etest.NewTransfer(t, env, deployment, sender, route)

	// Given started relayer
	relayer := e2etest.StartRelayer(t, driver, env)

	// Given packet1 sent and auto-relayed
	transfer1 := mustSend(t, transferApp, big.NewInt(100_000))
	mustDeliver(t, relayer, transfer1)

	// ACT #1: break the relayer's websocket subscription through the proxy.
	chainWSProxy.Kill()
	time.Sleep(time.Second)

	// ACT #2: send packet while relayer is disconnected.
	transfer2 := mustSend(t, transferApp, big.NewInt(200_000))

	// ACT #3: restore the proxy; the relayer resubscribes and triggers ws reconnection + clearing pass.
	chainWSProxy.Revive()
	time.Sleep(time.Second)

	// ACT/ASSERT #3: send packet2 after reconnect; the resubscribed watcher should pick it up.
	transfer3 := mustSend(t, transferApp, big.NewInt(300_000))
	mustDeliver(t, relayer, transfer3)
	mustDeliver(t, relayer, transfer2)
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

func mustSend(t *testing.T, app *e2etest.Transfer, amount *big.Int) *e2etest.TransferSend {
	t.Helper()

	ctx := t.Context()

	transfer, err := app.Send(ctx, e2etest.TransferRequest{Amount: amount})
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyEscrowed(ctx))

	return transfer
}

func mustDeliver(t *testing.T, relayer *ibccli.Relayer, transfer *e2etest.TransferSend) {
	t.Helper()

	ctx := t.Context()

	_, err := e2etest.AwaitState(ctx, relayer, transfer.PacketTx(), relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyDelivered(ctx))
}
