package e2e_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/link/cmd/relayercmd"
)

func TestRelayerRestart_ResumesPendingPacket(t *testing.T) {
	env := e2etest.Start(t, e2etest.SelectedSuite(t))
	signers := e2etest.NewSigners(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.Deploy(t, env, signers, route)
	transferApp := e2etest.BindTransfer(t, env, deployment, signers, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()
	amount := big.NewInt(777_000)

	require.NoError(t, relayer.Stop(ctx))

	transfer, err := transferApp.Send(ctx, e2etest.TransferRequest{Amount: amount})
	require.NoError(t, err)

	require.NoError(t, transfer.VerifyEscrowed(ctx))
	require.NoError(t, transfer.VerifyNotMinted(ctx))

	relayer = e2etest.StartRelayer(t, driver, env)
	destination, err := env.Chain(route.Destination)
	require.NoError(t, err)
	_, err = e2etest.AwaitState(
		ctx,
		relayer,
		transfer.Packet(),
		relayercmd.PacketComplete,
		destination.Timing(),
	)
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyDelivered(ctx))
}

func TestManualRelay_RequestSurvivesRestart(t *testing.T) {
	selected := e2etest.SelectedSuite(t)
	e2etest.RequireCapabilities(t, selected, environment.Requirements{
		MiningControl: []environment.ChainID{e2etest.ChainB},
	})
	env := e2etest.Start(t, selected)
	signers := e2etest.NewSigners(t)
	route := e2etest.ManualAtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.Deploy(t, env, signers, route)
	transferApp := e2etest.BindTransfer(t, env, deployment, signers, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	chainB, err := env.Chain(e2etest.ChainB)
	require.NoError(t, err)
	mining, err := chainB.Mining()
	require.NoError(t, err)
	dst, err := chainB.EVM()
	require.NoError(t, err)

	// Keep destination mining paused across restart so delivery cannot finish before the new Relayer is up.
	require.NoError(t, mining.WithPaused(ctx, func() error {
		transfer, err := transferApp.Send(ctx, e2etest.TransferRequest{Amount: big.NewInt(888_000)})
		require.NoError(t, err)
		require.NoError(t, transfer.VerifyEscrowed(ctx))

		require.NoError(t, e2etest.Relay(ctx, relayer, transfer.Packet()))
		require.NoError(t, relayer.Stop(ctx))
		relayer = e2etest.StartRelayer(t, driver, env)

		require.NoError(t, dst.WaitNextPendingTx(ctx))
		require.NoError(t, mining.Mine(ctx, 1))
		_, err = e2etest.AwaitState(
			ctx,
			relayer,
			transfer.Packet(),
			relayercmd.PacketComplete,
			chainB.Timing(),
		)
		require.NoError(t, err)
		require.NoError(t, transfer.VerifyDelivered(ctx))
		return nil
	}))
}
