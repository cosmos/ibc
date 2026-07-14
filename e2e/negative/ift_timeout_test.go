package negative_test

import (
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/ibclink"
	"github.com/cosmos/ibc/e2e/internal/synthetic"
	"github.com/cosmos/ibc/e2e/internal/testapp"
	"github.com/cosmos/ibc/link/cmd/relayercmd"
)

const (
	transferTimeout = 60 * time.Second
	// Advance well past transferTimeout so the timeout check wins any relay race.
	timeAdvance = 5 * transferTimeout
)

func TestIFTTimeout_Refund(t *testing.T) {
	selected := e2etest.SelectedSuite(t)
	e2etest.RequireCapabilities(t, selected, environment.Requirements{
		MiningControl: []environment.ChainID{e2etest.ChainB},
	})
	env := e2etest.Start(t, selected)
	signers := synthetic.NewSigners(t)
	route := synthetic.AtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := synthetic.Deploy(t, env, signers, route)
	ift := synthetic.BindIFT(t, env, deployment, signers, route)
	relayer := synthetic.StartRelayer(t, driver, env)
	ctx := t.Context()

	require.NoError(t, relayer.Stop(ctx))
	transfer, err := ift.Send(ctx, testapp.IFTRequest{
		Amount:  big.NewInt(3_000_000),
		Timeout: transferTimeout,
	})
	require.NoError(t, err)

	chainB, err := env.Chain(route.Destination)
	require.NoError(t, err)
	mining, err := chainB.Mining()
	require.NoError(t, err)
	require.NoError(t, mining.AdvanceTime(ctx, timeAdvance))
	relayer = synthetic.StartRelayer(t, driver, env)
	assertTimedOutAndRefunded(t, env, relayer, transfer)
}

func TestIFTTimeout_ManualRelayRefund(t *testing.T) {
	selected := e2etest.SelectedSuite(t)
	e2etest.RequireCapabilities(t, selected, environment.Requirements{
		MiningControl: []environment.ChainID{e2etest.ChainB},
	})
	env := e2etest.Start(t, selected)
	signers := synthetic.NewSigners(t)
	route := synthetic.ManualAtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := synthetic.Deploy(t, env, signers, route)
	ift := synthetic.BindIFT(t, env, deployment, signers, route)
	relayer := synthetic.StartRelayer(t, driver, env)
	ctx := t.Context()

	transfer, err := ift.Send(ctx, testapp.IFTRequest{
		Amount:  big.NewInt(3_000_000),
		Timeout: transferTimeout,
	})
	require.NoError(t, err)

	chainB, err := env.Chain(route.Destination)
	require.NoError(t, err)
	mining, err := chainB.Mining()
	require.NoError(t, err)
	require.NoError(t, mining.AdvanceTime(ctx, timeAdvance))
	require.NoError(t, synthetic.Relay(ctx, relayer, transfer.Packet()))
	assertTimedOutAndRefunded(t, env, relayer, transfer)
}

func assertTimedOutAndRefunded(
	t *testing.T,
	env *environment.Environment,
	relayer *ibclink.Relayer,
	transfer *testapp.IFTTransfer,
) {
	t.Helper()
	ctx := t.Context()
	packet := transfer.Packet()
	source, err := env.Chain(packet.Source)
	require.NoError(t, err)
	_, err = synthetic.AwaitState(
		ctx,
		relayer,
		packet,
		relayercmd.PacketTimedOut,
		source.Timing(),
	)
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyRefunded(ctx))
	require.NoError(t, transfer.VerifyNotMinted(ctx))
}
