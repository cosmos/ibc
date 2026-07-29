package e2e_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"

	relayerv2 "github.com/cosmos/ibc/link/api/v2/relayer"
)

// crossRouteSuite shares destination chain-a; both routes send sequence 1 to probe bare-seq collision.
func crossRouteSuite() e2etest.Suite {
	const (
		chainA environment.ChainID = "chain-a"
		chainB environment.ChainID = "chain-b"
		chainC environment.ChainID = "chain-c"
	)
	return e2etest.SuiteFor(environment.Spec{Chains: []environment.ChainSpec{
		environment.ManagedAnvil{ID: chainA, EVMChainID: 31637},
		environment.ManagedAnvil{ID: chainB, EVMChainID: 31638},
		environment.ManagedAnvil{ID: chainC, EVMChainID: 31639},
	}}, environment.Runtime{})
}

func TestCrossRoutePacketsDoNotCollideBySequence(t *testing.T) {
	t.Parallel()
	e2etest.RequireAnvilLane(t)
	env := e2etest.Start(t, crossRouteSuite())
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

	destination, err := env.Chain("chain-a")
	require.NoError(t, err)
	err = e2etest.AwaitState(ctx, relayer, bToA.Packet(),
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED, destination.Timing())
	require.NoError(t, err)
	err = e2etest.AwaitState(ctx, relayer, cToA.Packet(),
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED, destination.Timing())
	require.NoError(t, err)
	require.NoError(t, bToA.VerifyDelivered(ctx))
	require.NoError(t, cToA.VerifyDelivered(ctx))
}
