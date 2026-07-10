package negative_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/e2e/e2etest"
	"github.com/cosmos/ibc/link/harness"
	"github.com/cosmos/ibc/link/harness/topology"
)

func TestFault_NodeStop_DaemonRecovers(t *testing.T) {
	e2etest.RequireAnvilLane(t)
	run := e2etest.Start(t, topology.Anvil(topology.TwoEVM()))
	ctx := t.Context()

	chainB := run.Chain(topology.ChainB)
	dst, err := chainB.EVM()
	require.NoError(t, err)

	receiver, err := dst.NewFundedAccount(ctx)
	require.NoError(t, err)
	plan, err := run.PrepareIFT(ctx, harness.IFT{
		Route: topology.RouteAtoB, Amount: big.NewInt(2_000_000), Receiver: receiver.Addr.Hex(),
	})
	require.NoError(t, err)

	require.NoError(t, chainB.StopNode(ctx))
	_, err = dst.Height(ctx)
	require.Error(t, err, "with the destination node stopped, a harness height query must fail")

	out, err := plan.Submit(ctx)
	require.NoError(t, err)

	require.NoError(t, out.VerifyPendingStatus(ctx))
	require.NoError(t, out.VerifyPendingStable(ctx))

	require.NoError(t, chainB.StartNode(ctx))
	_, err = dst.Height(ctx)
	require.NoError(t, err, "after StartNode the destination must be reachable again")

	require.NoError(t, out.VerifyComplete(ctx))
}
