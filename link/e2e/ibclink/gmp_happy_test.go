package ibclink_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/e2e/e2etest"
	"github.com/cosmos/ibc/link/e2e/internal/synthetic"
	"github.com/cosmos/ibc/link/e2e/internal/testapp"
	"github.com/cosmos/ibc/link/harness/ibclink/wire"
)

func TestGMPCall_AutoRelay(t *testing.T) {
	routes := synthetic.Bidirectional(e2etest.ChainA, e2etest.ChainB)
	tests := []struct {
		name  string
		route synthetic.Route
	}{
		{"A_to_B", routes[0]},
		{"B_to_A", routes[1]},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := e2etest.Start(t, e2etest.SelectedSuite(t))
			signers := synthetic.NewSigners(t)
			driver, deployment := synthetic.Deploy(t, env, signers, routes...)
			gmp := synthetic.BindGMP(t, env, deployment, signers, tt.route)
			relayer := synthetic.StartRelayer(t, driver, env)
			ctx := t.Context()

			call, err := gmp.Call(ctx, testapp.GMPRequest{})
			require.NoError(t, err)
			destination, err := env.Chain(tt.route.Destination)
			require.NoError(t, err)
			_, err = synthetic.AwaitState(
				ctx,
				relayer,
				call.Packet(),
				wire.PacketComplete,
				destination.Timing(),
			)
			require.NoError(t, err)
			require.NoError(t, call.VerifyExecuted(ctx))
		})
	}
}
