// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	"context"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/ibclink"
	relayerv2 "github.com/cosmos/ibc/link/api/v2/relayer"
	"github.com/cosmos/ibc/link/lightclient/remotepoc"
)

// TestRemoteAttestationLightClientRelaysPacket relays through a remote prover.
func TestRemoteAttestationLightClientRelaysPacket(t *testing.T) {
	// Build the downstream CLI that registers the remote prover factory.
	t.Setenv("IBC_BIN", buildCustomIBC(t))
	ctx := t.Context()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	spec, runtime := attestedMesh(e2etest.EVMChains(
		t, e2etest.EVMRequirements{}, e2etest.ChainA, e2etest.ChainB,
	))
	env := e2etest.Start(t, spec, runtime)
	sender := e2etest.NewSigner(t)
	route := e2etest.ManualAtoB(e2etest.ChainA, e2etest.ChainB)
	var serviceConfig ibclink.RelayerConfig
	// Keep the attestation config for the proof service, but configure the
	// relayer to obtain those proofs from that service over HTTP.
	driver, deployment := e2etest.DeployWithRelayerConfig(
		t,
		env,
		sender,
		e2etest.NewSigner(t),
		func(cfg *ibclink.RelayerConfig) {
			require.NotEmpty(t, cfg.Connections)
			serviceConfig = cloneRelayerConfig(*cfg)
			cfg.Connections[0].ClientAType = remotepoc.Type
			cfg.Connections[0].ClientAParams = map[string]any{"url": "http://" + listener.Addr().String()}
		},
		route,
	)
	// Serve the built-in attestation prover behind the remote prover protocol.
	serveAttestationProver(t, listener, env, serviceConfig)

	// Relay a real packet through the custom-compiled CLI and remote prover.
	relayer := e2etest.StartRelayer(t, driver, env)
	transfer, err := e2etest.NewTransfer(t, env, deployment, sender, route).Send(
		ctx, e2etest.TransferRequest{Amount: big.NewInt(1_234_000)},
	)
	require.NoError(t, err)
	require.NoError(t, e2etest.RelayAll(ctx, relayer, transfer.PacketTx()))
	_, err = e2etest.AwaitState(
		ctx, relayer, transfer.PacketTx(), relayerv2.PacketState_PACKET_STATE_SUCCEEDED,
	)
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyDelivered(ctx))
}

func serveAttestationProver(
	t *testing.T,
	listener net.Listener,
	env *environment.Environment,
	cfg ibclink.RelayerConfig,
) {
	t.Helper()
	useEnvironmentRPCs(t, env, &cfg)
	configPath := filepath.Join(t.TempDir(), "attestation-proof-service.yaml")
	require.NoError(t, ibclink.WriteRelayerConfig(configPath, cfg))

	client := cfg.Connections[0]
	server, err := remotepoc.NewAttestationHandler(t.Context(), configPath, client.ChainA, client.ClientA)
	require.NoError(t, err)
	errs := make(chan error, 1)
	go func() { errs <- server.Serve(listener) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, server.Shutdown(ctx))
		require.ErrorIs(t, <-errs, http.ErrServerClosed)
	})
}

func cloneRelayerConfig(cfg ibclink.RelayerConfig) ibclink.RelayerConfig {
	cfg.Chains = append([]ibclink.RelayerChain(nil), cfg.Chains...)
	cfg.Connections = append([]ibclink.RelayerConnection(nil), cfg.Connections...)
	cfg.Attestors = append([]ibclink.RelayerAttestor(nil), cfg.Attestors...)
	return cfg
}

func useEnvironmentRPCs(
	t *testing.T, env *environment.Environment, cfg *ibclink.RelayerConfig,
) {
	t.Helper()
	for i := range cfg.Chains {
		for _, id := range env.Chains() {
			chain, err := env.Chain(id)
			require.NoError(t, err)
			if strconv.FormatUint(chain.EVMChainID(), 10) == cfg.Chains[i].ChainID {
				cfg.Chains[i].RPC = chain.RPCURL()
				break
			}
		}
		require.NotContains(t, cfg.Chains[i].RPC, "${")
	}
}

func buildCustomIBC(t *testing.T) string {
	t.Helper()

	binary := filepath.Join(t.TempDir(), "ibc")
	cmd := exec.CommandContext(t.Context(), "go", "build", "-o", binary, "./cmd/custom-ibc")
	cmd.Env = append(os.Environ(), "GOCACHE="+filepath.Join(t.TempDir(), "go-cache"))
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "build custom ibc binary:\n%s", output)

	return binary
}
