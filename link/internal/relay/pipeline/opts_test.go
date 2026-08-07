package pipeline

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/relay/processors"
	"github.com/cosmos/ibc/link/internal/relay/proofgen"
	"github.com/cosmos/ibc/link/internal/tests/mocks"
)

func TestOptionsFromConfigFinalityOffsets(t *testing.T) {
	route := processors.Route{
		SourceChainID:       "chain-a",
		SourceClientID:      "client-a",
		DestinationChainID:  "chain-b",
		DestinationClientID: "client-b",
	}

	cfg := config.Config{
		Relayer: config.RelayerConfig{
			Connections: []config.ConnectionConfig{
				{
					Alias:   "client-a-b",
					ClientA: config.ClientEnd{ClientID: "client-a", ChainID: "chain-a", Signer: "a-signer", Type: config.ClientTypeAttestation},
					ClientB: config.ClientEnd{ClientID: "client-b", ChainID: "chain-b", Signer: "b-signer", Type: config.ClientTypeAttestation},
				},
			},
		},
	}

	t.Run("eachClientsOwnCounterpartyMarginGatesItsCounterpartyChain", func(t *testing.T) {
		sourceGen := mocks.NewMockProofGenerator(t)
		sourceGen.EXPECT().FinalityOffset().Return(uint64(5))

		destGen := mocks.NewMockProofGenerator(t)
		destGen.EXPECT().FinalityOffset().Return(uint64(9))

		proofGenerators := staticProofGenerators{
			proofgen.Key(route.SourceChainID, route.SourceClientID):           sourceGen,
			proofgen.Key(route.DestinationChainID, route.DestinationClientID): destGen,
		}

		opts := OptionsFromConfig(cfg, proofGenerators, route)

		require.NotNil(t, opts.SourceFinalityOffset)
		require.Equal(
			t, uint64(9), *opts.SourceFinalityOffset,
			"the destination client's own resolved margin gates the source chain's send tx",
		)

		require.NotNil(t, opts.DestinationFinalityOffset)
		require.Equal(
			t, uint64(5), *opts.DestinationFinalityOffset,
			"the source client's own resolved margin gates destination chain state (timeouts, acks)",
		)
	})

	t.Run("noProofGeneratorLeavesOffsetsUnset", func(t *testing.T) {
		opts := OptionsFromConfig(cfg, staticProofGenerators{}, route)

		require.Nil(t, opts.SourceFinalityOffset)
		require.Nil(t, opts.DestinationFinalityOffset)
	})
}
