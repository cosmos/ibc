// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	relayerv2 "github.com/cosmos/ibc/cli/api/v2/relayer"
	"github.com/cosmos/ibc/e2e/internal/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/traffic"
)

func TestRelayerRecoversAfterNodeRestart(t *testing.T) {
	t.Parallel()
	spec, runtime := traffic.AttestedMesh(e2etest.EVMChains(t,
		e2etest.EVMRequirements{NodeLifecycle: true}, traffic.ChainA, traffic.ChainB)...)
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
	node, err := chainB.NodeLifecycle()
	require.NoError(t, err)

	prepared, err := transferApp.Prepare(ctx, traffic.TransferRequest{
		Amount: big.NewInt(2_000_000),
	})
	require.NoError(t, err)

	require.NoError(t, node.Stop(ctx))
	probeCtx, cancelProbe := context.WithTimeout(ctx, 2*time.Second)
	_, err = chainB.Height(probeCtx)
	cancelProbe()
	require.Error(t, err, "with the destination node stopped, an EVM height query must fail")

	transfer, err := prepared.Submit(ctx)
	require.NoError(t, err)

	require.NoError(t, node.Start(ctx))
	_, err = chainB.Height(ctx)
	require.NoError(t, err, "after node restart the destination must be reachable again")

	_, err = traffic.AwaitState(ctx, relayer, transfer.PacketTx(),
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyDelivered(ctx))
	require.NoError(t, transfer.VerifyEscrowed(ctx))
}
