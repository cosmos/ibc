// SPDX-License-Identifier: Apache-2.0

// Package txbuilder assembles transactions from packet relay and client
// update details. There is one implementation per supported chain type.
package txbuilder

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/config"
	"github.com/cosmos/ibc/link/internal/relay/txbuilder/evm"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

// TxBuilder builds transactions from packet relay and client update details
// for one chain. It returns a list of transactions rather than one, since
// some chains (e.g. Solana, due to tx size limits) must split a batch of
// packet relays across multiple transactions.
type TxBuilder interface {
	BuildRelayTxs(clientUpdate v2.ClientUpdate, packetRelayItems []v2.PacketRelayItem) ([]v2.RelayTx, error)
}

var _ TxBuilder = (*evm.TxBuilder)(nil)

// Set resolves a TxBuilder by chain id.
type Set struct {
	builders map[string]TxBuilder
}

func NewSet(builders map[string]TxBuilder) *Set {
	if builders == nil {
		builders = make(map[string]TxBuilder)
	}

	return &Set{builders: builders}
}

func (s *Set) Get(chainID string) (TxBuilder, bool) {
	builder, ok := s.builders[chainID]
	return builder, ok
}

// NewSetFromConfig builds a Set with one EVM TxBuilder per configured EVM
// chain, bound to that chain's configured ICS26Router address.
func NewSetFromConfig(cfg config.Config) (*Set, error) {
	builders := make(map[string]TxBuilder, len(cfg.Chains))

	for _, chainCfg := range cfg.Chains {
		switch chainCfg.Type() {
		case config.ChainTypeEVM:
			if !common.IsHexAddress(chainCfg.EVM.ICS26Router) {
				return nil, errors.Errorf(
					"chain %q: invalid ics26 router address %q",
					chainCfg.ChainID,
					chainCfg.EVM.ICS26Router,
				)
			}

			builders[chainCfg.ChainID] = evm.New(common.HexToAddress(chainCfg.EVM.ICS26Router))
		default:
			return nil, errors.Errorf(
				"chain %q: unsupported chain type %q for tx building",
				chainCfg.ChainID,
				chainCfg.Type(),
			)
		}
	}

	return NewSet(builders), nil
}
