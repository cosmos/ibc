package ibclink_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/e2etest"
	"github.com/cosmos/ibc/e2e/internal/synthetic"
	"github.com/cosmos/ibc/e2e/internal/testapp"
	"github.com/cosmos/ibc/link/cmd/relayercmd"
)

func TestGMPCall_ManualRelay(t *testing.T) {
	env := e2etest.Start(t, e2etest.SelectedSuite(t))
	signers := synthetic.NewSigners(t)
	route := synthetic.ManualAtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := synthetic.Deploy(t, env, signers, route)
	gmp := synthetic.BindGMP(t, env, deployment, signers, route)
	relayer := synthetic.StartRelayer(t, driver, env)
	ctx := t.Context()

	call, err := gmp.Call(ctx, testapp.GMPRequest{})
	require.NoError(t, err)

	destination, err := env.Chain(route.Destination)
	require.NoError(t, err)
	require.NoError(t, synthetic.AwaitStable(
		ctx,
		relayer,
		call.Packet(),
		relayercmd.PacketPending,
		destination.Timing(),
	))
	require.NoError(t, call.VerifyTargetUnchanged(ctx))
	require.NoError(t, synthetic.Relay(ctx, relayer, call.Packet()))
	_, err = synthetic.AwaitState(
		ctx,
		relayer,
		call.Packet(),
		relayercmd.PacketComplete,
		destination.Timing(),
	)
	require.NoError(t, err)
	require.NoError(t, call.VerifyExecuted(ctx))
}
