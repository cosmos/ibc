package negative_test

import (
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/e2e/e2etest"
	"github.com/cosmos/ibc/link/harness"
	"github.com/cosmos/ibc/link/harness/topology"
)

const (
	transferTimeout = 60 * time.Second
	// Advance well past transferTimeout so the stub's timeout check fires before relay races it.
	timeAdvance = 5 * transferTimeout
)

func TestIFTTimeout_Refund(t *testing.T) {
	e2etest.RequireAnvilLane(t)
	run := e2etest.Start(t, topology.Anvil(topology.TwoEVM()))
	ctx := t.Context()

	require.NoError(t, run.StopRelayer(ctx))

	out, err := run.IFT(ctx, harness.IFT{
		Route: topology.RouteAtoB, Amount: big.NewInt(3_000_000), Timeout: transferTimeout,
	})
	require.NoError(t, err)

	require.NoError(t, run.Chain(topology.ChainB).AdvanceTime(ctx, timeAdvance))
	require.NoError(t, run.RestartRelayer(ctx))
	require.NoError(t, out.VerifyTimedOut(ctx))
}

func TestIFTTimeout_ManualRelayRefund(t *testing.T) {
	e2etest.RequireAnvilLane(t)
	run := e2etest.Start(t, topology.Anvil(topology.TwoEVM()).WithManualRelay(topology.RouteAtoB))
	ctx := t.Context()

	out, err := run.IFT(ctx, harness.IFT{
		Route: topology.RouteAtoB, Amount: big.NewInt(3_000_000), Timeout: transferTimeout,
	})
	require.NoError(t, err)

	require.NoError(t, run.Chain(topology.ChainB).AdvanceTime(ctx, timeAdvance))
	require.NoError(t, out.Relay(ctx))
	require.NoError(t, out.VerifyTimedOut(ctx))
}
