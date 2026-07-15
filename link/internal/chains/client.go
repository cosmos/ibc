// Package chains defines chain-agnostic clients for reading chain state.
package chains

import (
	"context"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/chains/evm"
	"github.com/cosmos/ibc/link/internal/config"

	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

// Client provides chain state queries.
type Client interface {
	TxPacketEvents(ctx context.Context, txHash []byte) ([]v2.PacketEvent, error)
}

var _ Client = (*evm.Client)(nil)

// ClientSet holds the chain clients for all configured chains.
type ClientSet struct {
	clients map[string]Client
}

func NewClientSet(clients map[string]Client) *ClientSet {
	if clients == nil {
		clients = make(map[string]Client)
	}

	return &ClientSet{clients: clients}
}

func NewClientSetFromConfig(cfg config.Config) (*ClientSet, error) {
	clients := make(map[string]Client, len(cfg.Chains))

	for _, chain := range cfg.Chains {
		if chain.Type() != config.ChainTypeEVM {
			return nil, errors.Errorf("unsupported chain type for chain %q", chain.ChainID)
		}

		client, err := evm.New(chain.ChainID, chain.EVM.RPC, chain.EVM.ICS26Router)
		if err != nil {
			return nil, errors.Wrapf(err, "creating evm client for chain %q", chain.ChainID)
		}

		clients[chain.ChainID] = client
	}

	return NewClientSet(clients), nil
}

func (s *ClientSet) Get(chainID string) (Client, bool) {
	client, ok := s.clients[chainID]
	return client, ok
}
