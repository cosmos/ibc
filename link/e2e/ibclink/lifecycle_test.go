package ibclink_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/e2e/e2etest"
	"github.com/cosmos/ibc/link/harness"
	"github.com/cosmos/ibc/link/harness/topology"
)

func TestRestartRecovery_ResumesPendingPacket(t *testing.T) {
	run := e2etest.Start(t, e2etest.SelectedTopology(t))
	ctx := t.Context()
	amount := big.NewInt(777_000)

	require.NoError(t, run.StopRelayer(ctx))

	out, err := run.IFT(ctx, harness.IFT{Route: topology.RouteAtoB, Amount: amount})
	require.NoError(t, err)

	require.NoError(t, out.VerifyEscrowed(ctx))
	require.NoError(t, out.VerifyNoMint(ctx))

	require.NoError(t, run.RestartRelayer(ctx))
	require.NoError(t, out.VerifyComplete(ctx))
}

func TestManualRelay_RequestSurvivesRestart(t *testing.T) {
	e2etest.RequireAnvilLane(t)
	run := e2etest.Start(t, topology.Anvil(topology.TwoEVM()).WithManualRelay(topology.RouteAtoB))
	ctx := t.Context()

	chainB := run.Chain(topology.ChainB)
	dst, err := chainB.EVM()
	require.NoError(t, err)
	// The receiver is funded before mining pauses (funding a fresh account needs a mined tx).
	receiver, err := dst.NewFundedAccount(ctx)
	require.NoError(t, err)

	// Destination mining stays paused across the restart so delivery cannot complete before the new
	// daemon is up — completion afterwards proves the relay request was persisted, not already served.
	require.NoError(t, chainB.WithPausedMining(ctx, func() error {
		out, err := run.IFT(ctx, harness.IFT{
			Route: topology.RouteAtoB, Amount: big.NewInt(888_000), Receiver: receiver.Addr.Hex(),
		})
		require.NoError(t, err)

		require.NoError(t, out.Relay(ctx))
		require.NoError(t, run.RestartRelayer(ctx))

		require.NoError(t, dst.WaitNextPendingTx(ctx))
		require.NoError(t, chainB.Mine(ctx, 1))
		require.NoError(t, out.VerifyComplete(ctx))
		return nil
	}))
}
