package ibclink_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/e2e/e2etest"
	"github.com/cosmos/ibc/link/harness"
	"github.com/cosmos/ibc/link/harness/topology"
)

func TestPendingPacket_Anvil_StatusIsBetterSignal(t *testing.T) {
	e2etest.RequireAnvilLane(t)
	run := e2etest.Start(t, topology.Anvil(topology.TwoEVM()))
	ctx := t.Context()

	chainB := run.Chain(topology.ChainB)
	dst, err := chainB.EVM()
	require.NoError(t, err)
	receiver, err := dst.NewFundedAccount(ctx)
	require.NoError(t, err)

	require.NoError(t, chainB.WithPausedMining(ctx, func() error {
		out, err := run.IFT(ctx, harness.IFT{
			Route: topology.RouteAtoB, Amount: big.NewInt(424_242), Receiver: receiver.Addr.Hex(),
		})
		require.NoError(t, err)

		require.NoError(t, dst.WaitNextPendingTx(ctx))

		require.NoError(t, out.VerifyPending(ctx))
		require.NoError(t, out.VerifyPendingStable(ctx))

		require.NoError(t, chainB.Mine(ctx, 1))
		require.NoError(t, out.VerifyComplete(ctx))
		return nil
	}))
}
