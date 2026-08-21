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

// buildProverConfig is buildConfig's counterpart for the prover: a separate
// service with its own config, as it would be in a deployment. It needs the
// chains, attestors, and client ends it proves, and nothing else the relayer
// config carries.
func buildProverConfig(
	t testing.TB,
	env *environment.Environment,
	signerKeyPath string,
) ibclink.ProverConfig {
	t.Helper()

	config := ibclink.ProverConfig{
		DBPath:         filepath.Join(t.TempDir(), "prover.db"),
		SignerAlias:    relayerSignerAlias,
		SignerKeyFile:  signerKeyPath,
		FinalityOffset: ibclink.HarnessFinalityOffset,
	}

	// The prover dials chains directly, so its config carries the address
	// rather than the variable the relayer's process resolves.
	config.Chains = chainConfigs(t, env, func(id environment.ChainID) string {
		chain, err := env.Chain(id)
		require.NoError(t, err, "e2etest: resolve Chain %q", id)

		return chain.RPCURL()
	})
	config.Attestors = attestorConfigs(t, env)

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

// StartProver writes the prover's config and runs it.
func StartProver(t testing.TB, env *environment.Environment, signer Signer) *ibclink.Prover {
	t.Helper()

	dir := t.TempDir()
	signerKeyPath := filepath.Join(dir, "prover-signer.json")
	require.NoError(t, signer.storeKey(signerKeyPath), "e2etest: store prover signer key")

	config := buildProverConfig(t, env, signerKeyPath)
	configPath := filepath.Join(dir, "prover.config.yaml")
	require.NoError(t, ibclink.WriteProverConfig(configPath, config), "e2etest: write prover config")

	prover, err := ibclink.StartProver(configPath)
	require.NoError(t, err, "e2etest: start prover service")

	t.Cleanup(func() { assert.NoError(t, prover.Stop(), "e2etest: stop prover service") })

	return prover
}

func chainEVMID(t testing.TB, env *environment.Environment, id environment.ChainID) string {
	t.Helper()

	chain, err := env.Chain(id)
	require.NoError(t, err, "e2etest: resolve Chain %q", id)

	return strconv.FormatUint(chain.EVMChainID(), 10)
}

// chainConfigs describes every chain in the environment, taking each chain's
// RPC from rpcFor so the relayer and the prover can name it differently.
func chainConfigs(
	t testing.TB,
	env *environment.Environment,
	rpcFor func(environment.ChainID) string,
) []ibclink.RelayerChain {
	t.Helper()

	chains := make([]ibclink.RelayerChain, 0, len(env.Chains()))

	for _, id := range env.Chains() {
		instance, err := env.IBCInstanceForChain(id)
		require.NoError(t, err, "e2etest: resolve IBC instance for Chain %q", id)

		chains = append(chains, ibclink.RelayerChain{
			ChainID:     chainEVMID(t, env, id),
			RPC:         rpcFor(id),
			ICS26Router: string(instance.Locator()),
		})
	}

	return chains
}

// attestorConfigs describes every attestor in the environment. Attestors run as
// their own processes, so both the relayer and the prover reach them remotely.
func attestorConfigs(t testing.TB, env *environment.Environment) []ibclink.RelayerAttestor {
	t.Helper()

	attestors := make([]ibclink.RelayerAttestor, 0, len(env.Attestors()))

	for _, id := range env.Attestors() {
		attestor, err := env.Attestor(id)
		require.NoError(t, err, "e2etest: resolve Attestor %q", id)

		attestors = append(attestors, ibclink.RelayerAttestor{
			Name: string(attestor.ID()), Type: ibclink.RelayerAttestorRemote, GRPC: attestor.Endpoint(),
		})
	}

	return attestors
}
