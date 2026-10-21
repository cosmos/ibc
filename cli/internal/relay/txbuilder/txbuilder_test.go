// SPDX-License-Identifier: Apache-2.0

package txbuilder

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/internal/config"
)

func TestNewSetFromConfig(t *testing.T) {
	const router = "0x0000000000000000000000000000000000000001"
	const multicall = "0x0000000000000000000000000000000000000002"
	cfg := config.Config{}
	for _, chainID := range []string{"1", "2", "3"} {
		cfg.Chains = append(cfg.Chains, config.ChainConfig{
			ChainID: chainID,
			EVM:     &config.EVMChainConfig{RPC: "http://127.0.0.1:8545", ICS26Router: router, IBCMulticall: multicall},
		})
	}
	builders, err := NewSetFromConfig(cfg)
	require.NoError(t, err)
	for _, chainID := range []string{"1", "2", "3"} {
		_, ok := builders.Get(chainID)
		require.True(t, ok)
	}
	cfg.Chains[2].EVM.IBCMulticall = ""
	_, err = NewSetFromConfig(cfg)
	require.ErrorContains(t, err, `chain "3": invalid ibcMulticall`)
}
