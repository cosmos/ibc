package negative_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/e2e/e2etest"
	"github.com/cosmos/ibc/link/harness"
	"github.com/cosmos/ibc/link/harness/topology"
)

// badCalldata does not match MockGMP's expected target call, so delivery lands as an error ack.
var badCalldata = []byte{0xde, 0xad, 0xbe, 0xef}

func TestGMPErrorAck(t *testing.T) {
	run := e2etest.Start(t, e2etest.SelectedTopology(t))
	ctx := t.Context()

	out, err := run.GMP(ctx, harness.GMP{Route: topology.RouteAtoB, Payload: badCalldata})
	require.NoError(t, err)
	require.NoError(t, out.VerifyErrorAck(ctx))
}
