// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	"math/big"
	"net"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/ibclink"
	relayerv2 "github.com/cosmos/ibc/link/api/v2/relayer"
)

// freeLoopbackAddress reserves a port and releases it, so the relayer can be
// pointed at an address that is known before it starts.
func freeLoopbackAddress(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	address := listener.Addr().String()
	require.NoError(t, listener.Close())

	return address
}

// A custom light client is supported by implementing ProverService rather than
// by linking Go code into the relayer, so this relays a real packet with every
// proof fetched over gRPC.
//
// The service here is the relayer's own attestation prover, served over the
// same wire contract a third-party implementation would satisfy. In production
// the service is a separate process; the contract exercised is identical.
func TestRemoteProver_RelaysPacket(t *testing.T) {
	t.Parallel()

	spec, runtime := attestedMesh(e2etest.EVMChains(t, e2etest.EVMRequirements{}, e2etest.ChainA, e2etest.ChainB))
	env := e2etest.Start(t, spec, runtime)
	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSigner(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)

	proverAddress := freeLoopbackAddress(t)

	driver, deployment := e2etest.DeployWithRelayerConfig(t, env, sender, relayerSigner,
		func(cfg *ibclink.RelayerConfig) {
			cfg.ListenAddress = proverAddress
			for i := range cfg.Connections {
				cfg.Connections[i].ProverURL = "http://" + proverAddress
			}
		}, route)

	transferApp := e2etest.NewTransfer(t, env, deployment, sender, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	amount := new(big.Int).Mul(big.NewInt(500_000), big.NewInt(1_000_000_000_000_000_000))
	transfer, err := transferApp.Send(ctx, e2etest.TransferRequest{
		Amount: amount,
		Memo:   "remote-prover",
	})
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyEscrowed(ctx))

	status, err := e2etest.AwaitState(ctx, relayer, transfer.PacketTx(),
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)

	// The full lifecycle: every proof behind these was produced by the remote
	// service, so a wire contract that dropped or corrupted a field would fail
	// on chain rather than silently pass.
	require.NoError(t, transfer.VerifyDelivered(ctx))
	require.NoError(t, transfer.VerifyCommitmentCreated(ctx))
	require.NoError(t, transfer.VerifyReceiptCreated(ctx))
	require.NoError(t, transfer.VerifyCommitmentCleared(ctx))
	require.NoError(t, transfer.VerifyAcknowledgementWritten(ctx, status.GetRecvTx().GetTxHash()))
	require.NoError(t, transfer.VerifyAcknowledgementExecuted(ctx, status.GetAckTx().GetTxHash()))
}
