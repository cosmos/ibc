package negative_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/ibclink/wire"
	"github.com/cosmos/ibc/e2e/internal/synthetic"
)

func TestManualRelay_UnknownSourceTxErrors(t *testing.T) {
	env := e2etest.Start(t, e2etest.SelectedSuite(t))
	signers := synthetic.NewSigners(t)
	driver, _ := synthetic.Deploy(t, env, signers, synthetic.AtoB(e2etest.ChainA, e2etest.ChainB))
	relayer := synthetic.StartRelayer(t, driver, env)
	ctx := t.Context()

	_, err := relayer.Relay(ctx, wire.RelayRequest{
		SourceChainID: string(e2etest.ChainA),
		SourceTxHash:  "0x00000000000000000000000000000000000000000000000000000000deadbeef",
	})
	require.ErrorContains(t, err, "no packet found")
}
