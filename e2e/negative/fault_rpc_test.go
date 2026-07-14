package negative_test

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/synthetic"
	"github.com/cosmos/ibc/e2e/internal/testapp"
	"github.com/cosmos/ibc/link/cmd/relayercmd"
)

func TestFault_NodeStop_RelayerRecovers(t *testing.T) {
	selected := e2etest.SelectedSuite(t)
	e2etest.RequireCapabilities(t, selected, environment.Requirements{
		NodeLifecycle: []environment.ChainID{e2etest.ChainB},
	})
	env := e2etest.Start(t, selected)
	signers := synthetic.NewSigners(t)
	route := synthetic.AtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := synthetic.Deploy(t, env, signers, route)
	ift := synthetic.BindIFT(t, env, deployment, signers, route)
	relayer := synthetic.StartRelayer(t, driver, env)
	ctx := t.Context()

	chainB, err := env.Chain(e2etest.ChainB)
	require.NoError(t, err)
	node, err := chainB.NodeLifecycle()
	require.NoError(t, err)

	prepared, err := ift.Prepare(ctx, testapp.IFTRequest{
		Amount: big.NewInt(2_000_000),
	})
	require.NoError(t, err)

	require.NoError(t, node.Stop(ctx))
	probeCtx, cancelProbe := context.WithTimeout(ctx, 2*time.Second)
	_, err = chainB.Height(probeCtx)
	cancelProbe()
	require.Error(t, err, "with the destination node stopped, an EVM height query must fail")

	transfer, err := prepared.Submit(ctx)
	require.NoError(t, err)

	require.NoError(t, node.Start(ctx))
	_, err = chainB.Height(ctx)
	require.NoError(t, err, "after node restart the destination must be reachable again")

	_, err = synthetic.AwaitState(
		ctx,
		relayer,
		transfer.Packet(),
		relayercmd.PacketComplete,
		chainB.Timing(),
	)
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyDelivered(ctx))
	require.NoError(t, transfer.VerifyEscrowed(ctx))
}
