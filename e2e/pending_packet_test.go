package e2e_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/e2etest"
	"github.com/cosmos/ibc/link/cmd/relayercmd"
)

func TestPendingPacketStatusWhileDestinationMiningPaused(t *testing.T) {
	e2etest.RequireAnvilLane(t)
	env := e2etest.Start(t, e2etest.SelectedSuite(t))
	signers := e2etest.NewSigners(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.Deploy(t, env, signers, route)
	transferApp := e2etest.BindTransfer(t, env, deployment, signers, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	chainB, err := env.Chain(e2etest.ChainB)
	require.NoError(t, err)
	mining, err := chainB.Mining()
	require.NoError(t, err)

	var transfer *e2etest.TransferPacket
	require.NoError(t, mining.WithPaused(ctx, func() error {
		transfer, err = transferApp.Send(ctx, e2etest.TransferRequest{Amount: big.NewInt(424_242)})
		require.NoError(t, err)
		require.NoError(t, transfer.VerifyEscrowed(ctx))

		require.NoError(t, e2etest.AwaitStable(
			ctx,
			relayer,
			transfer.Packet(),
			relayercmd.PacketPending,
			chainB.Timing(),
		))
		require.NoError(t, transfer.VerifyNotMinted(ctx))
		return nil
	}))

	// The relayer only submits to a chain whose clock is current and only
	// counts blocks behind the tip as final, so delivery completes once the
	// destination produces blocks again.
	_, err = e2etest.AwaitState(
		ctx,
		relayer,
		transfer.Packet(),
		relayercmd.PacketComplete,
		chainB.Timing(),
	)
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyDelivered(ctx))
}
