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

func TestPendingPacketStatusWhileDestinationMiningPaused(t *testing.T) {
	selected := e2etest.SelectedSuite(t)
	e2etest.RequireCapabilities(t, selected, environment.Requirements{
		MiningControl: []environment.ChainID{e2etest.ChainB},
	})
	env := e2etest.Start(t, selected)
	signers := synthetic.NewSigners(t)
	route := synthetic.AtoB(e2etest.ChainA, e2etest.ChainB)
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

	require.NoError(t, mining.WithPaused(ctx, func() error {
		transfer, err := ift.Send(ctx, testapp.IFTRequest{Amount: big.NewInt(424_242)})
		require.NoError(t, err)
		require.NoError(t, transfer.VerifyEscrowed(ctx))

		require.NoError(t, dst.WaitNextPendingTx(ctx))

		require.NoError(t, synthetic.AwaitStable(
			ctx,
			relayer,
			transfer.Packet(),
			relayercmd.PacketPending,
			chainB.Timing(),
		))
		require.NoError(t, transfer.VerifyNotMinted(ctx))

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
