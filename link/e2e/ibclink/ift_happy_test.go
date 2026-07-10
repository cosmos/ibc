package ibclink_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/e2e/e2etest"
	"github.com/cosmos/ibc/link/harness"
	"github.com/cosmos/ibc/link/harness/topology"
)

func TestIFTTransfer_AutoRelay(t *testing.T) {
	run := e2etest.Start(t, e2etest.SelectedTopology(t))
	ctx := t.Context()

	out, err := run.IFT(ctx, harness.IFT{
		Route:  topology.RouteAtoB,
		Amount: big.NewInt(1_234_000),
	})
	require.NoError(t, err)
	require.NoError(t, out.VerifyComplete(ctx))
}
