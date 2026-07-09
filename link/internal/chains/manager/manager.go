// Package manager provides chain client lookup.
package manager

import (
	"context"
	"sync"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/chains/evm"
	"github.com/cosmos/ibc/link/internal/config"
)

// ClientManager holds the chain clients for all configured chains. Safe for concurrent use.
type ClientManager struct {
	mu      sync.RWMutex
	clients map[string]chains.Client
}

// New ClientManager constructor.
func New(clients map[string]chains.Client) *ClientManager {
	if clients == nil {
		clients = make(map[string]chains.Client)
	}

	return &ClientManager{clients: clients}
}

// NewFromConfig builds clients for all configured relayer chains.
func NewFromConfig(cfg config.Config) (*ClientManager, error) {
	clients := make(map[string]chains.Client)

	for _, relayerChain := range cfg.Relayer.Chains {
		if relayerChain.EVM == nil {
			continue
		}

		chain, ok := cfg.Chain(relayerChain.ChainID)
		if !ok {
			return nil, errors.Errorf("chain %q not declared in top-level chains", relayerChain.ChainID)
		}

		client, err := evm.New(relayerChain.ChainID, chain.EVM.RPC, relayerChain.EVM.Contracts.ICS26Router)
		if err != nil {
			return nil, errors.Wrapf(err, "creating evm client for chain %q", relayerChain.ChainID)
		}

		clients[relayerChain.ChainID] = client
	}

	return New(clients), nil
}

func (m *ClientManager) GetClient(_ context.Context, chainID string) (chains.Client, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	client, ok := m.clients[chainID]
	if !ok {
		return nil, errors.Errorf("no configured chain client for chain ID %s", chainID)
	}

	return client, nil
}
