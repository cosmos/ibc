// SPDX-License-Identifier: Apache-2.0

package e2etest

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/ibccli"
)

const proverStopTimeout = 15 * time.Second

// buildProverConfig covers every Connection in env, since the prover starts
// before Deploy and serves whichever routes the relayer is later given.
func buildProverConfig(
	t testing.TB,
	env *environment.Environment,
	signerKeyPath string,
) ibccli.RelayerConfig {
	t.Helper()

	config, chainIDs := baseRelayerConfig(t, env, signerKeyPath, filepath.Join(t.TempDir(), "prover.db"),
		func(_ environment.ChainID, chain *environment.Chain, relayerChain *ibccli.RelayerChain) {
			relayerChain.RPC = chain.RPCURL()
		})

	clientEnd := func(client *environment.IBCClient) ibccli.RelayerClientEnd {
		return ibccli.RelayerClientEnd{
			ChainID:    chainIDs[string(client.IBCInstance().Chain().ID())],
			ClientID:   client.ID(),
			ClientType: client.Kind(),
		}
	}
	for _, id := range env.Connections() {
		connection, err := env.Connection(id)
		require.NoError(t, err, "e2etest: resolve Connection %q", id)

		config.Connections = append(config.Connections, ibccli.RelayerConnection{
			A: clientEnd(connection.A()),
			B: clientEnd(connection.B()),
		})
	}

	return config
}

// StartProver writes the prover's config and runs it.
func StartProver(t testing.TB, env *environment.Environment, signer Signer) *ibccli.Prover {
	t.Helper()

	dir := t.TempDir()
	signerKeyPath := filepath.Join(dir, "prover-signer.json")
	require.NoError(t, signer.storeKey(signerKeyPath), "e2etest: store prover signer key")

	config := buildProverConfig(t, env, signerKeyPath)
	configPath := filepath.Join(dir, "prover.config.yaml")
	require.NoError(t, ibccli.WriteRelayerConfig(configPath, config), "e2etest: write prover config")

	prover, err := ibccli.StartProver(t.Context(), configPath)
	require.NoError(t, err, "e2etest: start prover service")

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), proverStopTimeout)
		defer cancel()
		assert.NoError(t, prover.Stop(ctx), "e2etest: stop prover service")
	})

	return prover
}
