package e2e_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	relayerv2 "github.com/cosmos/ibc/link/api/v2/relayer"
)

// crossRouteSpec shares destination chain-a; both routes send sequence 1 to probe bare-seq collision.
func crossRouteSpec(t testing.TB) environment.Spec {
	t.Helper()
	const (
		chainA environment.ChainID = "chain-a"
		chainB environment.ChainID = "chain-b"
		chainC environment.ChainID = "chain-c"
	)
	return dummyClientMeshSpec(e2etest.EVMChains(t, e2etest.EVMRequirements{}, chainA, chainB, chainC))
}

func TestCrossRoutePacketsDoNotCollideBySequence(t *testing.T) {
	t.Parallel()
	spec := crossRouteSpec(t)
	runtime := e2etest.RuntimeWithProtocolDeployer(environment.Runtime{})
	env := e2etest.Start(t, spec, runtime)
	signers := e2etest.NewSigners(t)
	routes := []e2etest.Route{
		{ID: "b-to-a", Source: "chain-b", Destination: "chain-a"},
		{ID: "c-to-a", Source: "chain-c", Destination: "chain-a"},
	}
	driver, deployment := e2etest.Deploy(t, env, signers, routes...)
	bToAApp := e2etest.BindTransfer(t, env, deployment, signers, routes[0])
	cToAApp := e2etest.BindTransfer(t, env, deployment, signers, routes[1])
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	bToA, err := bToAApp.Send(ctx, e2etest.TransferRequest{Amount: big.NewInt(333)})
	require.NoError(t, err)
	require.NoError(t, bToA.VerifyEscrowed(ctx))
	cToA, err := cToAApp.Send(ctx, e2etest.TransferRequest{Amount: big.NewInt(444)})
	require.NoError(t, err)
	require.NoError(t, cToA.VerifyEscrowed(ctx))

	_, err = e2etest.AwaitState(ctx, relayer, bToA.Packet(),
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)
	_, err = e2etest.AwaitState(ctx, relayer, cToA.Packet(),
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)
	require.NoError(t, bToA.VerifyDelivered(ctx))
	require.NoError(t, cToA.VerifyDelivered(ctx))
}
