// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	relayerv2 "github.com/cosmos/ibc/cli/api/v2/relayer"
	"github.com/cosmos/ibc/e2e/internal/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/traffic"
)

func TestPendingPacketStatusWhileDestinationMiningPaused(t *testing.T) {
	t.Parallel()
	spec, runtime := traffic.AttestedMesh(e2etest.EVMChains(t,
		e2etest.EVMRequirements{ControlledMining: true}, traffic.ChainA, traffic.ChainB)...)
	env := e2etest.Start(t, spec, runtime)
	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSigner(t)
	route := traffic.AtoB(traffic.ChainA, traffic.ChainB)
	driver, deployment := e2etest.Deploy(t, env, sender, relayerSigner, route)
	transferApp, err := traffic.NewTransfer(env, deployment, sender, route)
	require.NoError(t, err)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	chainB, err := env.Chain(traffic.ChainB)
	require.NoError(t, err)
	mining, err := chainB.Mining()
	require.NoError(t, err)

	var transfer *traffic.TransferSend
	require.NoError(t, mining.WithPaused(ctx, func() error {
		transfer, err = transferApp.Send(ctx, traffic.TransferRequest{Amount: big.NewInt(424_242)})
		require.NoError(t, err)
		require.NoError(t, transfer.VerifyEscrowed(ctx))

		require.NoError(t, traffic.AwaitStable(ctx, relayer, transfer.PacketTx(),
			relayerv2.PacketState_PACKET_STATE_PENDING))
		require.NoError(t, transfer.VerifyNotMinted(ctx))
		return nil
	}))

	// The relayer only submits to a chain whose clock is current and only
	// counts blocks behind the tip as final, so delivery completes once the
	// destination produces blocks again.
	_, err = traffic.AwaitState(ctx, relayer, transfer.PacketTx(),
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyDelivered(ctx))
}
