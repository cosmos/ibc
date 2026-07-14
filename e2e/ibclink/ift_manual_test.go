package ibclink_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/ibclink/wire"
	"github.com/cosmos/ibc/e2e/internal/synthetic"
	"github.com/cosmos/ibc/e2e/internal/testapp"
)

func TestIFTTransfer_ManualRelay(t *testing.T) {
	env := e2etest.Start(t, e2etest.SelectedSuite(t))
	signers := synthetic.NewSigners(t)
	route := synthetic.ManualAtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := synthetic.Deploy(t, env, signers, route)
	ift := synthetic.BindIFT(t, env, deployment, signers, route)
	relayer := synthetic.StartRelayer(t, driver, env)
	ctx := t.Context()

	transfer, err := ift.Send(ctx, testapp.IFTRequest{Amount: big.NewInt(1_234_000)})
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyEscrowed(ctx))

	destination, err := env.Chain(route.Destination)
	require.NoError(t, err)
	require.NoError(t, synthetic.AwaitStable(
		ctx,
		relayer,
		transfer.Packet(),
		wire.PacketPending,
		destination.Timing(),
	))
	require.NoError(t, transfer.VerifyNotMinted(ctx))
	require.NoError(t, synthetic.Relay(ctx, relayer, transfer.Packet()))
	_, err = synthetic.AwaitState(
		ctx,
		relayer,
		transfer.Packet(),
		wire.PacketComplete,
		destination.Timing(),
	)
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyDelivered(ctx))
}
