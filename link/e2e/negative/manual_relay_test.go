package negative_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/e2e/e2etest"
	"github.com/cosmos/ibc/link/harness"
	"github.com/cosmos/ibc/link/harness/topology"
)

func TestManualRelay_UnknownSourceTxErrors(t *testing.T) {
	run := e2etest.Start(t, e2etest.SelectedTopology(t))
	ctx := t.Context()

	err := run.Relay(ctx, harness.Relay{
		SourceChainID: topology.ChainA,
		SourceTxHash:  "0x00000000000000000000000000000000000000000000000000000000deadbeef",
	})
	require.ErrorContains(t, err, "no packet found")
}
