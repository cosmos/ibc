package ibclink_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/ibclink/wire"
	"github.com/cosmos/ibc/e2e/internal/synthetic"
	"github.com/cosmos/ibc/e2e/internal/testapp"
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

func TestCrossRoute_NoSequenceCollision(t *testing.T) {
	e2etest.RequireAnvilLane(t)
	env := e2etest.Start(t, crossRouteSuite())
	signers := synthetic.NewSigners(t)
	routes := []synthetic.Route{
		{ID: "b-to-a", Source: "chain-b", Destination: "chain-a"},
		{ID: "c-to-a", Source: "chain-c", Destination: "chain-a"},
	}
	driver, deployment := synthetic.Deploy(t, env, signers, routes...)
	bToAApp := synthetic.BindIFT(t, env, deployment, signers, routes[0])
	cToAApp := synthetic.BindIFT(t, env, deployment, signers, routes[1])
	relayer := synthetic.StartRelayer(t, driver, env)
	ctx := t.Context()

	bToA, err := bToAApp.Send(ctx, testapp.IFTRequest{Amount: big.NewInt(333)})
	require.NoError(t, err)
	require.NoError(t, bToA.VerifyEscrowed(ctx))
	cToA, err := cToAApp.Send(ctx, testapp.IFTRequest{Amount: big.NewInt(444)})
	require.NoError(t, err)
	require.NoError(t, cToA.VerifyEscrowed(ctx))

	destination, err := env.Chain("chain-a")
	require.NoError(t, err)
	_, err = synthetic.AwaitState(ctx, relayer, bToA.Packet(), wire.PacketComplete, destination.Timing())
	require.NoError(t, err)
	_, err = synthetic.AwaitState(ctx, relayer, cToA.Packet(), wire.PacketComplete, destination.Timing())
	require.NoError(t, err)
	require.NoError(t, bToA.VerifyDelivered(ctx))
	require.NoError(t, cToA.VerifyDelivered(ctx))
}
