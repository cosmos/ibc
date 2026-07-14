package negative_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/ibclink/wire"
	"github.com/cosmos/ibc/e2e/internal/synthetic"
	"github.com/cosmos/ibc/e2e/internal/testapp"
)

// badCalldata does not match Counter's call surface, so delivery produces an error acknowledgement.
var badCalldata = []byte{0xde, 0xad, 0xbe, 0xef}

func TestGMPErrorAck(t *testing.T) {
	env := e2etest.Start(t, e2etest.SelectedSuite(t))
	signers := synthetic.NewSigners(t)
	route := synthetic.AtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := synthetic.Deploy(t, env, signers, route)
	gmp := synthetic.BindGMP(t, env, deployment, signers, route)
	relayer := synthetic.StartRelayer(t, driver, env)
	ctx := t.Context()

	call, err := gmp.Call(ctx, testapp.GMPRequest{Payload: badCalldata})
	require.NoError(t, err)
	destination, err := env.Chain(route.Destination)
	require.NoError(t, err)
	_, err = synthetic.AwaitState(
		ctx,
		relayer,
		call.Packet(),
		wire.PacketErrorAck,
		destination.Timing(),
	)
	require.NoError(t, err)
	require.NoError(t, call.VerifyRejected(ctx))
}
