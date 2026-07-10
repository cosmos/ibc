package cosmos_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/e2e/e2etest"
	"github.com/cosmos/ibc/link/harness"
	"github.com/cosmos/ibc/link/harness/topology"
)

// TestIFTTransfer_CosmosToEVM runs the REVERSE direction — the first non-EVM source — through the
// unchanged harness surface: the transfer escrows on the real cosmos source and mints on the EVM destination.
func TestIFTTransfer_CosmosToEVM(t *testing.T) {
	e2etest.RequireAnvilLane(t)
	e2etest.RequireSandboxd(t)

	run := e2etest.Start(t, topology.Anvil(topology.EVMCosmos()))
	ctx := t.Context()

	out, err := run.IFT(ctx, harness.IFT{
		Route:  topology.RouteBtoA,
		Amount: big.NewInt(3_141_592),
	})
	require.NoError(t, err)
	// The native source burn and packet commitment already committed at submit, so assert the sharper
	// cosmos-source leg cheaply before the full completion floor.
	require.NoError(t, out.VerifyEscrowed(ctx))
	require.NoError(t, out.VerifyComplete(ctx))
}
