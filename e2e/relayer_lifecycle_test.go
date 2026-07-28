package e2e_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/e2etest"

	relayerv2 "github.com/cosmos/ibc/link/api/v2/relayer"
)

func TestManualRelay_RequestSurvivesRestart(t *testing.T) {
	t.Parallel()
	e2etest.RequireAnvilLane(t)
	env := e2etest.Start(t, e2etest.SelectedSuite(t))
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

	// Keep destination mining paused across restart so delivery cannot finish before the new Relayer is up.
	var transfer *e2etest.TransferPacket
	require.NoError(t, mining.WithPaused(ctx, func() error {
		transfer, err = transferApp.Send(ctx, e2etest.TransferRequest{Amount: big.NewInt(888_000)})
		require.NoError(t, err)
		require.NoError(t, transfer.VerifyEscrowed(ctx))

		require.NoError(t, e2etest.Relay(ctx, relayer, transfer.Packet()))
		require.NoError(t, relayer.Stop(ctx))
		relayer = e2etest.StartRelayer(t, driver, env)

		// The restarted relayer still tracks the packet from its store.
		require.NoError(t, e2etest.AwaitStable(ctx, relayer, transfer.Packet(),
			relayerv2.PacketState_PACKET_STATE_PENDING, chainB.Timing()))
		require.NoError(t, transfer.VerifyNotMinted(ctx))
		return nil
	}))

	err = e2etest.AwaitState(ctx, relayer, transfer.Packet(),
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED, chainB.Timing())
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyDelivered(ctx))
}
