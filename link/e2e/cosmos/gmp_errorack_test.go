package cosmos_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/e2e/e2etest"
	"github.com/cosmos/ibc/link/harness"
	"github.com/cosmos/ibc/link/harness/topology"
)

// badGMPPayload matches neither destination family's recognized call, so delivery lands as an error
// acknowledgement with the target left unchanged.
var badGMPPayload = []byte{0xde, 0xad, 0xbe, 0xef}

// TestGMPErrorAck_EVMToCosmos submits an a->b evmToCosmos GMP with an unrecognized payload; the 27-gmp
// module's atomic execution aborts and yields the universal error acknowledgement.
func TestGMPErrorAck_EVMToCosmos(t *testing.T) {
	e2etest.RequireAnvilLane(t)
	e2etest.RequireSandboxd(t)

	run := e2etest.Start(t, topology.Anvil(topology.EVMCosmos()))
	ctx := t.Context()

	out, err := run.GMP(ctx, harness.GMP{Route: topology.RouteAtoB, Payload: badGMPPayload})
	require.NoError(t, err)
	require.NoError(t, out.VerifyErrorAck(ctx))
}

// TestGMPErrorAck_CosmosToEVM submits the reverse b->a cosmosToEvm GMP with an unrecognized payload; the
// EVM delivery reverts and the relayer marks the packet error_ack.
func TestGMPErrorAck_CosmosToEVM(t *testing.T) {
	e2etest.RequireAnvilLane(t)
	e2etest.RequireSandboxd(t)

	run := e2etest.Start(t, topology.Anvil(topology.EVMCosmos()))
	ctx := t.Context()

	out, err := run.GMP(ctx, harness.GMP{Route: topology.RouteBtoA, Payload: badGMPPayload})
	require.NoError(t, err)
	require.NoError(t, out.VerifyErrorAck(ctx))
}
