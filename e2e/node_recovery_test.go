package e2e_test

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"

	relayerv2 "github.com/cosmos/ibc/link/api/v2/relayer"
)

func TestRelayerRecoversAfterNodeRestart(t *testing.T) {
	t.Parallel()
	e2etest.RequireAnvilNodeLifecycle(t)
	spec := dummyClientMeshSpec(e2etest.ChainSpecsForConfiguredLane(t))
	runtime := e2etest.RuntimeWithProtocolDeployer(environment.Runtime{})
	env := e2etest.Start(t, spec, runtime)
	signers := e2etest.NewSigners(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.Deploy(t, env, signers, route)
	transferApp := e2etest.BindTransfer(t, env, deployment, signers, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	chainB, err := env.Chain(e2etest.ChainB)
	require.NoError(t, err)
	node, err := chainB.NodeLifecycle()
	require.NoError(t, err)

	prepared, err := transferApp.Prepare(ctx, e2etest.TransferRequest{
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

	_, err = e2etest.AwaitState(ctx, relayer, transfer.Packet(),
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED, chainB.Timing())
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyDelivered(ctx))
	require.NoError(t, transfer.VerifyEscrowed(ctx))
}
