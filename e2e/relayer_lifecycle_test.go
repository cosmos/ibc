package e2e_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/synthetic"
	"github.com/cosmos/ibc/e2e/internal/testapp"
	"github.com/cosmos/ibc/link/cmd/relayercmd"
)

func TestRelayerRestart_ResumesPendingPacket(t *testing.T) {
	env := e2etest.Start(t, e2etest.SelectedSuite(t))
	signers := synthetic.NewSigners(t)
	route := synthetic.AtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := synthetic.Deploy(t, env, signers, route)
	ift := synthetic.BindIFT(t, env, deployment, signers, route)
	relayer := synthetic.StartRelayer(t, driver, env)
	ctx := t.Context()
	amount := big.NewInt(777_000)

	require.NoError(t, relayer.Stop(ctx))

	transfer, err := ift.Send(ctx, testapp.IFTRequest{Amount: amount})
	require.NoError(t, err)

	require.NoError(t, transfer.VerifyEscrowed(ctx))
	require.NoError(t, transfer.VerifyNotMinted(ctx))

	relayer = synthetic.StartRelayer(t, driver, env)
	destination, err := env.Chain(route.Destination)
	require.NoError(t, err)
	_, err = synthetic.AwaitState(
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
	signers := synthetic.NewSigners(t)
	route := synthetic.ManualAtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := synthetic.Deploy(t, env, signers, route)
	ift := synthetic.BindIFT(t, env, deployment, signers, route)
	relayer := synthetic.StartRelayer(t, driver, env)
	ctx := t.Context()

	chainB, err := env.Chain(e2etest.ChainB)
	require.NoError(t, err)
	mining, err := chainB.Mining()
	require.NoError(t, err)
	dst, err := chainB.EVM()
	require.NoError(t, err)

	// Keep destination mining paused across restart so delivery cannot finish before the new Relayer is up.
	require.NoError(t, mining.WithPaused(ctx, func() error {
		transfer, err := ift.Send(ctx, testapp.IFTRequest{Amount: big.NewInt(888_000)})
		require.NoError(t, err)
		require.NoError(t, transfer.VerifyEscrowed(ctx))

		require.NoError(t, synthetic.Relay(ctx, relayer, transfer.Packet()))
		require.NoError(t, relayer.Stop(ctx))
		relayer = synthetic.StartRelayer(t, driver, env)

		require.NoError(t, dst.WaitNextPendingTx(ctx))
		require.NoError(t, mining.Mine(ctx, 1))
		_, err = synthetic.AwaitState(
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
