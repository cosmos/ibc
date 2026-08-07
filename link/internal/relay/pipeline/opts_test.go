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
				Connections: []config.ConnectionConfig{
					{
						Alias: "client-a-b",
						ClientA: config.ClientEnd{
							ClientID:    "client-a",
							ChainID:     "chain-a",
							Signer:      "a-signer",
							Type:        config.ClientTypeAttestation,
							AttestorSet: &config.AttestorSetConfig{CounterpartyChainFinalityOffset: 5},
						},
						ClientB: config.ClientEnd{
							ClientID:    "client-b",
							ChainID:     "chain-b",
							Signer:      "b-signer",
							Type:        config.ClientTypeAttestation,
							AttestorSet: &config.AttestorSetConfig{CounterpartyChainFinalityOffset: 9},
						},
					},
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
				Connections: []config.ConnectionConfig{
					{
						Alias:   "client-a-b",
						ClientA: config.ClientEnd{ClientID: "client-a", ChainID: "chain-a"},
						ClientB: config.ClientEnd{ClientID: "client-b", ChainID: "chain-b"},
					},
				},
			},
		}

		opts := OptionsFromConfig(cfg, route)

		require.Nil(t, opts.SourceFinalityOffset)
		require.Nil(t, opts.DestinationFinalityOffset)
	})
}
