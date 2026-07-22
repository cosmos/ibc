package e2e_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/link/cmd/relayercmd"
)

func TestIFTTransfer_AutoRelay(t *testing.T) {
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
			iftApp := e2etest.BindIFT(t, env, deployment, signers, tt.route)
			relayer := e2etest.StartRelayer(t, driver, env)
			ctx := t.Context()

			transfer, err := iftApp.Send(ctx, e2etest.IFTRequest{Amount: big.NewInt(1_234_000)})
			require.NoError(t, err)
			require.NoError(t, transfer.VerifyBurned(ctx))

			destination, err := env.Chain(tt.route.Destination)
			require.NoError(t, err)
			_, err = e2etest.AwaitState(
				ctx,
				relayer,
				transfer.Packet(),
				relayercmd.PacketComplete,
				destination.Timing(),
			)
			require.NoError(t, err)
			require.NoError(t, transfer.VerifyDelivered(ctx))
		})
	}
}

func TestIFTTimeout_Refund(t *testing.T) {
	selected := e2etest.SelectedSuite(t)
	e2etest.RequireCapabilities(t, selected, environment.Requirements{
		MiningControl: []environment.ChainID{e2etest.ChainB},
	})
	env := e2etest.Start(t, selected)
	signers := e2etest.NewSigners(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.Deploy(t, env, signers, route)
	iftApp := e2etest.BindIFT(t, env, deployment, signers, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	require.NoError(t, relayer.Stop(ctx))
	transfer, err := iftApp.Send(ctx, e2etest.IFTRequest{
		Amount:  big.NewInt(3_000_000),
		Timeout: transferTimeout,
	})
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyBurned(ctx))

	chainB, err := env.Chain(route.Destination)
	require.NoError(t, err)
	mining, err := chainB.Mining()
	require.NoError(t, err)
	require.NoError(t, mining.AdvanceTime(ctx, transferTimeoutAdvance))
	relayer = e2etest.StartRelayer(t, driver, env)

	source, err := env.Chain(route.Source)
	require.NoError(t, err)
	_, err = e2etest.AwaitState(
		ctx,
		relayer,
		transfer.Packet(),
		relayercmd.PacketTimedOut,
		source.Timing(),
	)
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyRefunded(ctx))
	require.NoError(t, transfer.VerifyNotMinted(ctx))
}
