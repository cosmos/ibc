package e2e_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/link/cmd/relayercmd"
)

func TestPendingPacketStatusWhileDestinationMiningPaused(t *testing.T) {
	selected := e2etest.SelectedSuite(t)
	e2etest.RequireCapabilities(t, selected, environment.Requirements{
		MiningControl: []environment.ChainID{e2etest.ChainB},
	})
	env := e2etest.Start(t, selected)
	signers := e2etest.NewSigners(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.Deploy(t, env, signers, route)
	ift := e2etest.BindIFT(t, env, deployment, signers, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	chainB, err := env.Chain(e2etest.ChainB)
	require.NoError(t, err)
	mining, err := chainB.Mining()
	require.NoError(t, err)
	dst, err := chainB.EVM()
	require.NoError(t, err)

	require.NoError(t, mining.WithPaused(ctx, func() error {
		transfer, err := ift.Send(ctx, e2etest.IFTRequest{Amount: big.NewInt(424_242)})
		require.NoError(t, err)
		require.NoError(t, transfer.VerifyEscrowed(ctx))

		require.NoError(t, dst.WaitNextPendingTx(ctx))

		require.NoError(t, e2etest.AwaitStable(
			ctx,
			relayer,
			transfer.Packet(),
			relayercmd.PacketPending,
			chainB.Timing(),
		))
		require.NoError(t, transfer.VerifyNotMinted(ctx))

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
