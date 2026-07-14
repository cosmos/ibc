package configcmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestCommandRunsInjectedValidateHandler(t *testing.T) {
	called := false
	cmd := NewCommand(Handlers{Validate: func(_ *cobra.Command, _ []string, options ValidateOptions) error {
		called = true
		require.True(t, options.Live)
		require.True(t, options.Strict)
		return nil
	}})
	cmd.SetArgs([]string{"validate", "--live", "--strict"})

	require.NoError(t, cmd.Execute())
	require.True(t, called)
}

func TestConfigYAMLSerialization(t *testing.T) {
	config := Config{
		Chains:  []Chain{{ID: "chain-a", Type: ChainTypeEVM, ChainID: 31337, RPC: RPC{URL: "http://rpc"}}},
		Signers: []Signer{},
		DB:      DB{Type: DBTypeSQLite, URL: "ibc.db"},
		Relayer: Relayer{Routes: []Route{{
			ID: "a-to-b", Source: "chain-a", Destination: "chain-b", Type: RouteEVMToEVMAttested,
		}}},
	}

	encoded, err := config.Marshal()
	require.NoError(t, err)
	require.Equal(t, strings.TrimSpace(`
chains:
    - id: chain-a
      type: evm
      chainId: 31337
      rpc:
        url: http://rpc
signers: []
db:
    type: sqlite
    url: ibc.db
relayer:
    routes:
        - id: a-to-b
          source: chain-a
          destination: chain-b
          type: evmToEvmAttested
`)+"\n", string(encoded))

	decoded, err := Unmarshal(encoded)
	require.NoError(t, err)
	require.Equal(t, config, *decoded)
}
