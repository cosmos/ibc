// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	relayerv2 "github.com/cosmos/ibc/link/api/v2/relayer"
)

func TestCrossRoutePacketsDoNotCollideBySequence(t *testing.T) {
	t.Parallel()
	const (
		chainA environment.ChainID = "chain-a"
		chainB environment.ChainID = "chain-b"
		chainC environment.ChainID = "chain-c"
	)
	// Both routes share destination chain-a and send sequence 1 to probe
	// bare-seq collision.
	spec, runtime := attestedMesh(e2etest.EVMChains(t, e2etest.EVMRequirements{}, chainA, chainB, chainC))
	env := e2etest.Start(t, spec, runtime)
	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSigner(t)
	routes := []e2etest.Route{
		{ID: "b-to-a", Source: chainB, Destination: chainA},
		{ID: "c-to-a", Source: chainC, Destination: chainA},
	}
	driver, deployment := e2etest.Deploy(t, env, sender, relayerSigner, routes...)
	bToAApp := e2etest.NewTransfer(t, env, deployment, sender, routes[0])
	cToAApp := e2etest.NewTransfer(t, env, deployment, sender, routes[1])
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	bToA, err := bToAApp.Send(ctx, e2etest.TransferRequest{Amount: big.NewInt(333)})
	require.NoError(t, err)
	require.NoError(t, bToA.VerifyEscrowed(ctx))
	cToA, err := cToAApp.Send(ctx, e2etest.TransferRequest{Amount: big.NewInt(444)})
	require.NoError(t, err)
	require.NoError(t, cToA.VerifyEscrowed(ctx))

	_, err = e2etest.AwaitState(ctx, relayer, bToA.PacketTx(),
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)
	_, err = e2etest.AwaitState(ctx, relayer, cToA.PacketTx(),
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)
	require.NoError(t, bToA.VerifyDelivered(ctx))
	require.NoError(t, cToA.VerifyDelivered(ctx))
}
