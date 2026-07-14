package stub

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/cmd/configcmd"
)

const (
	signerTestSource       = "source"
	signerTestMissingAlias = "missing"
)

func TestRelaySignerKeysRequireOnlyRouteEndpoints(t *testing.T) {
	c := &configcmd.Config{
		Chains: []configcmd.Chain{
			{ID: "unused", Type: configcmd.ChainTypeEVM},
			{ID: signerTestSource, Type: configcmd.ChainTypeEVM},
		},
		Relayer: configcmd.Relayer{Routes: []configcmd.Route{{Source: signerTestSource}}},
	}

	_, err := relaySignerKeys(c)
	require.ErrorContains(t, err, "chains[1].evmSigner: EVM relay signer alias is empty")
}

func TestRelaySignerKeysIdentifyUnknownAlias(t *testing.T) {
	c := &configcmd.Config{
		Chains: []configcmd.Chain{{
			ID: signerTestSource, Type: configcmd.ChainTypeEVM, EVMSigner: signerTestMissingAlias,
		}},
		Relayer: configcmd.Relayer{Routes: []configcmd.Route{{Source: signerTestSource}}},
	}

	_, err := relaySignerKeys(c)
	require.ErrorContains(t, err, `chains[0].evmSigner: EVM relay signer "missing"`)
	require.ErrorContains(t, err, `signer alias "missing" is not configured`)
}
