// SPDX-License-Identifier: Apache-2.0

package pipeline

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/relay/processors"
)

func TestOptionsFromConfigFinalityOffsets(t *testing.T) {
	route := processors.Route{
		SourceChainID:       "chain-a",
		SourceClientID:      "client-a",
		DestinationChainID:  "chain-b",
		DestinationClientID: "client-b",
	}

	t.Run("eachClientsOwnCounterpartyMarginGatesItsCounterpartyChain", func(t *testing.T) {
		cfg := config.Config{
			Relayer: config.RelayerConfig{
				Clients: []config.ClientConfig{
					{
						Alias:                "client-a",
						ClientID:             "client-a",
						ChainID:              "chain-a",
						CounterpartyChainID:  "chain-b",
						CounterpartyClientID: "client-b",
						Type:                 config.ClientTypeAttestation,
						AttestorSet:          &config.AttestorSetConfig{CounterpartyChainFinalityOffset: 5},
					},
					{
						Alias:                "client-b",
						ClientID:             "client-b",
						ChainID:              "chain-b",
						CounterpartyChainID:  "chain-a",
						CounterpartyClientID: "client-a",
						Type:                 config.ClientTypeAttestation,
						AttestorSet:          &config.AttestorSetConfig{CounterpartyChainFinalityOffset: 9},
					},
				},
				Routes: []config.RouteConfig{
					{SourceClient: "client-a", SourceSignerAlias: "a-signer", DestSignerAlias: "b-signer"},
				},
			},
		}

		opts := OptionsFromConfig(cfg, route)

		require.NotNil(t, opts.SourceFinalityOffset)
		require.Equal(
			t, uint64(9), *opts.SourceFinalityOffset,
			"the destination client's own counterparty margin gates the source chain's send tx",
		)

		require.NotNil(t, opts.DestinationFinalityOffset)
		require.Equal(
			t, uint64(5), *opts.DestinationFinalityOffset,
			"the source client's own counterparty margin gates destination chain state (timeouts, acks)",
		)
	})

	t.Run("noAttestorSetLeavesOffsetsUnset", func(t *testing.T) {
		cfg := config.Config{
			Relayer: config.RelayerConfig{
				Clients: []config.ClientConfig{
					{Alias: "client-a", ClientID: "client-a", ChainID: "chain-a"},
					{Alias: "client-b", ClientID: "client-b", ChainID: "chain-b"},
				},
			},
		}

		opts := OptionsFromConfig(cfg, route)

		require.Nil(t, opts.SourceFinalityOffset)
		require.Nil(t, opts.DestinationFinalityOffset)
	})
}
