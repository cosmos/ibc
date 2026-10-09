// SPDX-License-Identifier: Apache-2.0

package traffic

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/ibccli"
)

func buildProverConfig(
	env *environment.Environment,
	signerKeyPath string,
	dbPath string,
) (ibccli.RelayerConfig, error) {
	config := ibccli.RelayerConfig{
		DBPath:         dbPath,
		SignerAlias:    relayerSignerAlias,
		SignerKeyFile:  signerKeyPath,
		FinalityOffset: ibccli.HarnessFinalityOffset,
	}

	for _, id := range env.Chains() {
		chain, err := env.Chain(id)
		if err != nil {
			return config, fmt.Errorf("traffic: resolve Chain %q: %w", id, err)
		}

		instance, err := env.IBCInstanceForChain(id)
		if err != nil {
			return config, fmt.Errorf("traffic: resolve IBC instance for Chain %q: %w", id, err)
		}

		config.Chains = append(config.Chains, ibccli.RelayerChain{
			ChainID:     strconv.FormatUint(chain.EVMChainID(), 10),
			RPC:         chain.RPCURL(),
			ICS26Router: string(instance.Locator()),
		})
	}

	for _, id := range env.Attestors() {
		attestor, err := env.Attestor(id)
		if err != nil {
			return config, fmt.Errorf("traffic: resolve Attestor %q: %w", id, err)
		}

		config.Attestors = append(config.Attestors, ibccli.RelayerAttestor{
			Name: string(attestor.ID()), Type: ibccli.RelayerAttestorRemote, GRPC: attestor.Endpoint(),
		})
	}

	for _, id := range env.Connections() {
		connection, err := env.Connection(id)
		if err != nil {
			return config, fmt.Errorf("traffic: resolve Connection %q: %w", id, err)
		}

		chainA, err := chainEVMID(env, connection.A().IBCInstance().Chain().ID())
		if err != nil {
			return config, err
		}
		chainB, err := chainEVMID(env, connection.B().IBCInstance().Chain().ID())
		if err != nil {
			return config, err
		}
		config.Connections = append(config.Connections, ibccli.RelayerConnection{
			A: ibccli.RelayerClientEnd{
				ChainID:    chainA,
				ClientID:   connection.A().ID(),
				ClientType: connection.A().Kind(),
			},
			B: ibccli.RelayerClientEnd{
				ChainID:    chainB,
				ClientID:   connection.B().ID(),
				ClientType: connection.B().Kind(),
			},
		})
	}

	return config, nil
}

// StartProver writes the prover's config, key, and database under dir and
// runs it. The caller owns the returned prover and stops it with Stop before
// removing dir.
func StartProver(env *environment.Environment, signer Signer, dir string) (*ibccli.Prover, error) {
	if env == nil {
		return nil, errors.New("traffic: Environment is required")
	}
	if dir == "" {
		return nil, errors.New("traffic: prover directory is required")
	}

	signerKeyPath := filepath.Join(dir, "prover-signer.json")
	if err := signer.storeKey(signerKeyPath); err != nil {
		return nil, fmt.Errorf("traffic: store prover signer key: %w", err)
	}

	config, err := buildProverConfig(env, signerKeyPath, filepath.Join(dir, "prover.db"))
	if err != nil {
		return nil, err
	}
	configPath := filepath.Join(dir, "prover.config.yaml")
	if err = ibccli.WriteRelayerConfig(configPath, config); err != nil {
		return nil, fmt.Errorf("traffic: write prover config: %w", err)
	}

	prover, err := ibccli.StartProver(configPath)
	if err != nil {
		return nil, fmt.Errorf("traffic: start prover service: %w", err)
	}
	return prover, nil
}

func chainEVMID(env *environment.Environment, id environment.ChainID) (string, error) {
	chain, err := env.Chain(id)
	if err != nil {
		return "", fmt.Errorf("traffic: resolve Chain %q: %w", id, err)
	}
	return strconv.FormatUint(chain.EVMChainID(), 10), nil
}
