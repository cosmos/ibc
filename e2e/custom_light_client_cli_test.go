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

	"github.com/cosmos/ibc/e2e/internal/customlightclient"
	"github.com/cosmos/ibc/e2e/internal/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/ibclink"
	relayerv2 "github.com/cosmos/ibc/link/api/v2/relayer"
	"github.com/cosmos/ibc/link/lightclient/remotepoc"
)

// TestCustomCompiledCLILoadsLightClient proves that a downstream binary can
// register a light-client implementation and select it through the ordinary
// `ibc relayer run` path.
func TestCustomCompiledCLILoadsLightClient(t *testing.T) {
	binary := buildCustomIBC(t)
	t.Setenv("IBC_BIN", binary)

	spec, runtime := attestedMesh(e2etest.EVMChains(
		t, e2etest.EVMRequirements{}, e2etest.ChainA, e2etest.ChainB,
	))
	env := e2etest.Start(t, spec, runtime)

	marker := filepath.Join(t.TempDir(), "custom-client-created")
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	driver, _ := e2etest.DeployWithRelayerConfig(
		t,
		env,
		e2etest.NewSigner(t),
		e2etest.NewSigner(t),
		func(cfg *ibclink.RelayerConfig) {
			require.NotEmpty(t, cfg.Connections)
			cfg.Connections[0].ClientAType = customlightclient.Type
			cfg.Connections[0].ClientAParams = map[string]any{"markerFile": marker}
		},
		route,
	)

	relayer := e2etest.StartRelayer(t, driver, env)
	require.NotEmpty(t, relayer.Ready().HTTP)

	createdFor, err := os.ReadFile(marker)
	require.NoError(t, err, "custom light-client factory was not invoked")
	require.NotEmpty(t, createdFor)
}

// TestRemoteAttestationLightClientRelaysPacket proves a custom-compiled CLI can
// complete a packet relay using the built-in attestation generator through the
// remote light-client HTTP adapter.
func TestRemoteAttestationLightClientRelaysPacket(t *testing.T) {
	binary := buildCustomIBC(t)
	t.Setenv("IBC_BIN", binary)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	remoteURL := "http://" + listener.Addr().String()

	spec, runtime := attestedMesh(e2etest.EVMChains(
		t, e2etest.EVMRequirements{}, e2etest.ChainA, e2etest.ChainB,
	))
	env := e2etest.Start(t, spec, runtime)
	sender := e2etest.NewSigner(t)
	route := e2etest.ManualAtoB(e2etest.ChainA, e2etest.ChainB)
	var serviceConfig ibclink.RelayerConfig
	driver, deployment := e2etest.DeployWithRelayerConfig(
		t,
		env,
		sender,
		e2etest.NewSigner(t),
		func(cfg *ibclink.RelayerConfig) {
			require.NotEmpty(t, cfg.Connections)
			serviceConfig = cloneRelayerConfig(*cfg)
			cfg.Connections[0].ClientAType = remotepoc.Type
			cfg.Connections[0].ClientAParams = map[string]any{"url": remoteURL}
		},
		route,
	)

	useEnvironmentRPCs(t, env, &serviceConfig)
	serviceConfigPath := filepath.Join(t.TempDir(), "attestation-proof-service.yaml")
	require.NoError(t, ibclink.WriteRelayerConfig(serviceConfigPath, serviceConfig))
	remoteEnd := serviceConfig.Connections[0]
	server, err := remotepoc.NewAttestationHandler(
		t.Context(), serviceConfigPath, remoteEnd.ChainA, remoteEnd.ClientA,
	)
	require.NoError(t, err)
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.Serve(listener) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, server.Shutdown(ctx))
		serveErr := <-serverErrors
		require.ErrorIs(t, serveErr, http.ErrServerClosed)
	})

	relayer := e2etest.StartRelayer(t, driver, env)
	transfer, err := e2etest.NewTransfer(t, env, deployment, sender, route).Send(
		t.Context(), e2etest.TransferRequest{Amount: big.NewInt(1_234_000)},
	)
	require.NoError(t, err)
	require.NoError(t, e2etest.RelayAll(t.Context(), relayer, transfer.PacketTx()))
	_, err = e2etest.AwaitState(
		t.Context(), relayer, transfer.PacketTx(), relayerv2.PacketState_PACKET_STATE_SUCCEEDED,
	)
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyDelivered(t.Context()))
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
