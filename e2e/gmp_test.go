package e2e_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/e2etest"
	"github.com/cosmos/ibc/link/cmd/relayercmd"
)

func TestGMPCall_AutoRelay(t *testing.T) {
	routes := e2etest.Bidirectional(e2etest.ChainA, e2etest.ChainB)
	tests := []struct {
		name  string
		route e2etest.Route
	}{
		{"A_to_B", routes[0]},
		{"B_to_A", routes[1]},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := e2etest.Start(t, e2etest.SelectedSuite(t))
			signers := e2etest.NewSigners(t)
			driver, deployment := e2etest.Deploy(t, env, signers, routes...)
			gmp := e2etest.BindGMP(t, env, deployment, signers, tt.route)
			relayer := e2etest.StartRelayer(t, driver, env)
			ctx := t.Context()

			call, err := gmp.Call(ctx, e2etest.GMPRequest{})
			require.NoError(t, err)
			destination, err := env.Chain(tt.route.Destination)
			require.NoError(t, err)
			_, err = e2etest.AwaitState(
				ctx,
				relayer,
				call.Packet(),
				relayercmd.PacketComplete,
				destination.Timing(),
			)
			require.NoError(t, err)
			require.NoError(t, call.VerifyExecuted(ctx))
		})
	}
}

func TestGMPCall_ManualRelay(t *testing.T) {
	env := e2etest.Start(t, e2etest.SelectedSuite(t))
	signers := e2etest.NewSigners(t)
	route := e2etest.ManualAtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.Deploy(t, env, signers, route)
	gmp := e2etest.BindGMP(t, env, deployment, signers, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	call, err := gmp.Call(ctx, e2etest.GMPRequest{})
	require.NoError(t, err)

	destination, err := env.Chain(route.Destination)
	require.NoError(t, err)
	require.NoError(t, e2etest.AwaitStable(
		ctx,
		relayer,
		call.Packet(),
		relayercmd.PacketPending,
		destination.Timing(),
	))
	require.NoError(t, call.VerifyTargetUnchanged(ctx))
	require.NoError(t, e2etest.Relay(ctx, relayer, call.Packet()))
	_, err = e2etest.AwaitState(
		ctx,
		relayer,
		call.Packet(),
		relayercmd.PacketComplete,
		destination.Timing(),
	)
	require.NoError(t, err)
	require.NoError(t, call.VerifyExecuted(ctx))
}

// invalidGMPPayload does not match Counter's call surface, so delivery produces an error acknowledgement.
var invalidGMPPayload = []byte{0xde, 0xad, 0xbe, 0xef}

func TestGMPCall_ErrorAcknowledgement(t *testing.T) {
	env := e2etest.Start(t, e2etest.SelectedSuite(t))
	signers := e2etest.NewSigners(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.Deploy(t, env, signers, route)
	gmp := e2etest.BindGMP(t, env, deployment, signers, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	call, err := gmp.Call(ctx, e2etest.GMPRequest{Payload: invalidGMPPayload})
	require.NoError(t, err)
	destination, err := env.Chain(route.Destination)
	require.NoError(t, err)
	_, err = e2etest.AwaitState(
		ctx,
		relayer,
		call.Packet(),
		relayercmd.PacketErrorAck,
		destination.Timing(),
	)
	require.NoError(t, err)
	require.NoError(t, call.VerifyRejected(ctx))
}
