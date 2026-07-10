package cosmos_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/e2e/e2etest"
	"github.com/cosmos/ibc/link/harness"
	"github.com/cosmos/ibc/link/harness/topology"
)

// TestGMPCall_EVMToCosmos runs the a->b evmToCosmos GMP happy path with a defaulted target + payload,
// delivered for REAL over IBC v2 into the chain's native 27-gmp module.
func TestGMPCall_EVMToCosmos(t *testing.T) {
	e2etest.RequireAnvilLane(t)
	e2etest.RequireSandboxd(t)

	run := e2etest.Start(t, topology.Anvil(topology.EVMCosmos()))
	ctx := t.Context()

	out, err := run.GMP(ctx, harness.GMP{Route: topology.RouteAtoB})
	require.NoError(t, err)
	require.NoError(t, out.VerifyComplete(ctx))
}

// TestGMPCall_CosmosToEVM runs the reverse b->a cosmosToEvm GMP happy path: a real 27-gmp MsgSendCall
// on the cosmos source, delivered on the EVM destination via the same path evm->evm uses.
func TestGMPCall_CosmosToEVM(t *testing.T) {
	e2etest.RequireAnvilLane(t)
	e2etest.RequireSandboxd(t)

	run := e2etest.Start(t, topology.Anvil(topology.EVMCosmos()))
	ctx := t.Context()

	out, err := run.GMP(ctx, harness.GMP{Route: topology.RouteBtoA})
	require.NoError(t, err)
	require.NoError(t, out.VerifyComplete(ctx))
}
