// SPDX-License-Identifier: Apache-2.0

package e2etest

import (
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/ibclink"
)

// ProverConfig builds the prover service's own configuration from the
// environment. The prover is a separate service with a separate config, as it
// would be in a deployment; StartRelayer's config must agree with it, which
// AssertProverConfigMatches checks.
func ProverConfig(t testing.TB, env *environment.Environment, signerKeyPath string) ibclink.RelayerConfig {
	t.Helper()

	config := ibclink.RelayerConfig{
		DBPath:         filepath.Join(t.TempDir(), "prover.db"),
		SignerAlias:    relayerSignerAlias,
		SignerKeyFile:  signerKeyPath,
		FinalityOffset: ibclink.HarnessFinalityOffset,
	}

	for _, id := range env.Chains() {
		chain, err := env.Chain(id)
		require.NoError(t, err, "e2etest: resolve Chain %q", id)

		instance, err := env.IBCInstanceForChain(id)
		require.NoError(t, err, "e2etest: resolve IBC instance for Chain %q", id)

		config.Chains = append(config.Chains, ibclink.RelayerChain{
			ChainID:     strconv.FormatUint(chain.EVMChainID(), 10),
			RPC:         chain.RPCURL(),
			ICS26Router: string(instance.Locator()),
		})
	}

	for _, id := range env.Attestors() {
		attestor, err := env.Attestor(id)
		require.NoError(t, err, "e2etest: resolve Attestor %q", id)

		config.Attestors = append(config.Attestors, ibclink.RelayerAttestor{
			Name: string(attestor.ID()), Type: ibclink.RelayerAttestorRemote, GRPC: attestor.Endpoint(),
		})
	}

	for _, id := range env.Connections() {
		connection, err := env.Connection(id)
		require.NoError(t, err, "e2etest: resolve Connection %q", id)

		config.Connections = append(config.Connections, ibclink.RelayerConnection{
			ChainA:  chainEVMID(t, env, connection.A().IBCInstance().Chain().ID()),
			ClientA: connection.A().ID(),
			ChainB:  chainEVMID(t, env, connection.B().IBCInstance().Chain().ID()),
			ClientB: connection.B().ID(),
		})
	}

	return config
}

// StartProver writes the prover's config and runs it, returning the service
// and the config it was built from.
func StartProver(
	t testing.TB,
	env *environment.Environment,
	signer Signer,
) (*ibclink.Prover, ibclink.RelayerConfig) {
	t.Helper()

	dir := t.TempDir()
	signerKeyPath := filepath.Join(dir, "prover-signer.json")
	require.NoError(t, signer.storeKey(signerKeyPath), "e2etest: store prover signer key")

	config := ProverConfig(t, env, signerKeyPath)
	configPath := filepath.Join(dir, "prover.config.yaml")
	require.NoError(t, ibclink.WriteRelayerConfig(configPath, config), "e2etest: write prover config")

	prover, err := ibclink.StartProver(configPath)
	require.NoError(t, err, "e2etest: start prover service")

	t.Cleanup(func() { assert.NoError(t, prover.Stop(), "e2etest: stop prover service") })

	return prover, config
}

// AssertProverConfigMatches fails if the prover and the relayer disagree about
// the clients being proven, the chains they live on, or the attestors backing
// them. They are separate configs, so nothing but this stops them drifting.
func AssertProverConfigMatches(t testing.TB, prover, relayer ibclink.RelayerConfig) {
	t.Helper()

	assert.ElementsMatch(t, chainIdentities(relayer), chainIdentities(prover),
		"prover and relayer disagree about chains")
	assert.ElementsMatch(t, attestorIdentities(relayer), attestorIdentities(prover),
		"prover and relayer disagree about attestors")
	assert.ElementsMatch(t, clientIdentities(relayer), clientIdentities(prover),
		"prover and relayer disagree about clients")
}

func chainIdentities(cfg ibclink.RelayerConfig) []string {
	out := make([]string, 0, len(cfg.Chains))
	for _, chain := range cfg.Chains {
		out = append(out, chain.ChainID+"/"+chain.ICS26Router)
	}

	return out
}

func attestorIdentities(cfg ibclink.RelayerConfig) []string {
	out := make([]string, 0, len(cfg.Attestors))
	for _, attestor := range cfg.Attestors {
		out = append(out, attestor.Name+"/"+attestor.GRPC)
	}

	return out
}

func clientIdentities(cfg ibclink.RelayerConfig) []string {
	out := make([]string, 0, len(cfg.Connections)*2)
	for _, conn := range cfg.Connections {
		out = append(out,
			conn.ChainA+"/"+conn.ClientA,
			conn.ChainB+"/"+conn.ClientB,
		)
	}

	return out
}

func chainEVMID(t testing.TB, env *environment.Environment, id environment.ChainID) string {
	t.Helper()

	chain, err := env.Chain(id)
	require.NoError(t, err, "e2etest: resolve Chain %q", id)

	return strconv.FormatUint(chain.EVMChainID(), 10)
}
