package e2e_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/e2etest"
	"github.com/cosmos/ibc/link/cmd/relayercmd"
)

func TestManualRelayRejectsUnknownSourceTransaction(t *testing.T) {
	env := e2etest.Start(t, e2etest.SelectedSuite(t))
	signers := e2etest.NewSigners(t)
	driver, _ := e2etest.Deploy(t, env, signers, e2etest.AtoB(e2etest.ChainA, e2etest.ChainB))
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	_, err := relayer.Relay(ctx, relayercmd.RelayRequest{
		SourceChainID: string(e2etest.ChainA),
		SourceTxHash:  "0x00000000000000000000000000000000000000000000000000000000deadbeef",
	})
	require.ErrorContains(t, err, "no packet found")
}
