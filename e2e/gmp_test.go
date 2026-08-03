package e2e_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"

	relayerv2 "github.com/cosmos/ibc/link/api/v2/relayer"
)

func TestGMPCall_AutoRelay(t *testing.T) {
	t.Parallel()
	spec := dummyClientMeshSpec(e2etest.ChainSpecsForConfiguredLane(t))
	runtime := e2etest.RuntimeWithProtocolDeployer(environment.Runtime{})
	env := e2etest.Start(t, spec, runtime)
	signers := e2etest.NewSigners(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.Deploy(t, env, signers, route)
	gmp := e2etest.BindGMP(t, env, deployment, signers, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	call, err := gmp.Call(ctx, e2etest.GMPRequest{})
	require.NoError(t, err)
	destination, err := env.Chain(route.Destination)
	require.NoError(t, err)
	_, err = e2etest.AwaitState(ctx, relayer, call.Packet(),
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED, destination.Timing())
	require.NoError(t, err)
	require.NoError(t, call.VerifyExecuted(ctx))
}

// invalidGMPPayload does not match Counter's call surface, so delivery produces an error acknowledgement.
var invalidGMPPayload = []byte{0xde, 0xad, 0xbe, 0xef}

func TestGMPCall_ErrorAcknowledgement(t *testing.T) {
	t.Parallel()
	spec := dummyClientMeshSpec(e2etest.ChainSpecsForConfiguredLane(t))
	runtime := e2etest.RuntimeWithProtocolDeployer(environment.Runtime{})
	env := e2etest.Start(t, spec, runtime)
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
	_, err = e2etest.AwaitState(ctx, relayer, call.Packet(),
		relayerv2.PacketState_PACKET_STATE_REJECTED, destination.Timing())
	require.NoError(t, err)
	require.NoError(t, call.VerifyRejected(ctx))
}
