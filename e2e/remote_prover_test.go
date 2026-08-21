// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	"math/big"
	"net"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/ibclink"
	relayerv2 "github.com/cosmos/ibc/link/api/v2/relayer"
)

// startProverService runs the prover binary against the relayer's config. It
// is a separate process: the relayer holds only the endpoint.
func startProverService(t *testing.T, driver *ibclink.Driver, address string) {
	t.Helper()

	// The config expands chain RPCs from the environment, which the relayer
	// process is given; the prover needs the same.
	vars, release, err := driver.ChainRPCEnv()
	require.NoError(t, err, "resolve chain rpc env")

	t.Cleanup(release)

	env := os.Environ()
	for name, value := range vars {
		env = append(env, name+"="+value)
	}

	cmd := exec.Command(ibclink.ResolvedProverBin(),
		"--config", driver.ConfigPath(), "--listen", address)
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr

	require.NoError(t, cmd.Start(), "start prover service")

	t.Cleanup(func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		_ = cmd.Wait()
	})

	// The relayer must not ask for a proof before the service answers.
	require.Eventually(t, func() bool {
		conn, dialErr := net.DialTimeout("tcp", address, time.Second)
		if dialErr != nil {
			return false
		}

		_ = conn.Close()

		return true
	}, 30*time.Second, 100*time.Millisecond, "prover service did not start listening")
}

// reserveLoopbackAddress picks a port, since the config names it before the
// service starts.
func reserveLoopbackAddress(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	address := listener.Addr().String()
	require.NoError(t, listener.Close())

	return address
}

// Relays a real packet with every proof fetched over gRPC from a service the
// relayer does not host.
func TestRemoteProver_RelaysPacket(t *testing.T) {
	t.Parallel()

	spec, runtime := attestedMesh(e2etest.EVMChains(t, e2etest.EVMRequirements{}, e2etest.ChainA, e2etest.ChainB))
	env := e2etest.Start(t, spec, runtime)
	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSigner(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)

	proverAddress := reserveLoopbackAddress(t)

	driver, deployment := e2etest.DeployWithRelayerConfig(t, env, sender, relayerSigner,
		func(cfg *ibclink.RelayerConfig) {
			for i := range cfg.Connections {
				cfg.Connections[i].ProverURL = "http://" + proverAddress
			}
		}, route)

	// The service must answer before the relayer asks it for a proof.
	startProverService(t, driver, proverAddress)

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

	// Every proof came from the remote service, so a dropped field fails here.
	require.NoError(t, transfer.VerifyDelivered(ctx))
	require.NoError(t, transfer.VerifyCommitmentCreated(ctx))
	require.NoError(t, transfer.VerifyReceiptCreated(ctx))
	require.NoError(t, transfer.VerifyCommitmentCleared(ctx))
	require.NoError(t, transfer.VerifyAcknowledgementWritten(ctx, status.GetRecvTx().GetTxHash()))
	require.NoError(t, transfer.VerifyAcknowledgementExecuted(ctx, status.GetAckTx().GetTxHash()))
}
