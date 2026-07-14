package relay

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"
)

const (
	signerTestSource       = "source"
	signerTestMissingAlias = "missing"
)

func TestRelaySignerKeysRequireOnlyRouteEndpoints(t *testing.T) {
	c := &wire.ConfigYAML{
		Chains: []wire.Chain{
			{ID: "unused", Type: wire.ChainTypeEVM},
			{ID: signerTestSource, Type: wire.ChainTypeEVM},
		},
		Relayer: wire.Relayer{Routes: []wire.Route{{Source: signerTestSource}}},
	}

	_, err := relaySignerKeys(c)
	require.ErrorContains(t, err, "chains[1].evmSigner: EVM relay signer alias is empty")
}

func TestRelaySignerKeysIdentifyUnknownAlias(t *testing.T) {
	c := &wire.ConfigYAML{
		Chains: []wire.Chain{{
			ID: signerTestSource, Type: wire.ChainTypeEVM, EVMSigner: signerTestMissingAlias,
		}},
		Relayer: wire.Relayer{Routes: []wire.Route{{Source: signerTestSource}}},
	}

	_, err := relaySignerKeys(c)
	require.ErrorContains(t, err, `chains[0].evmSigner: EVM relay signer "missing"`)
	require.ErrorContains(t, err, `signer alias "missing" is not configured`)
}
