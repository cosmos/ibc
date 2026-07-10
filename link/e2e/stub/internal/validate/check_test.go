package validate

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"
)

const relayerKeyHex = "59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"

func goodConfig() *wire.ConfigYAML {
	return &wire.ConfigYAML{
		Chains: []wire.Chain{
			{
				ID:           "31337",
				Type:         "evm",
				Provider:     wire.ProviderAnvil,
				ChainID:      31337,
				EVMSignerKey: relayerKeyHex,
				RPC:          wire.RPC{URL: "http://127.0.0.1:8545"},
			},
			{
				ID:           "31338",
				Type:         "evm",
				Provider:     wire.ProviderAnvil,
				ChainID:      31338,
				EVMSignerKey: relayerKeyHex,
				RPC:          wire.RPC{URL: "http://127.0.0.1:8546"},
			},
		},
		DB: wire.DB{Type: wire.DBTypeSQLite, URL: "/tmp/relayer.db"},
		Relayer: wire.Relayer{
			Routes: []wire.Route{
				{ID: "route-a-to-b", Source: "31337", Destination: "31338", Type: "evmToEvmAttested"},
			},
		},
	}
}

func TestCheck_Valid(t *testing.T) {
	require.Empty(t, Check(goodConfig()), "a well-formed config has no structural errors")
}

func TestCheck_Providers(t *testing.T) {
	tests := []struct {
		name      string
		provider  string
		wantValid bool
	}{
		{name: "empty", provider: "", wantValid: false},
		{name: "anvil", provider: wire.ProviderAnvil, wantValid: true},
		{name: "besu", provider: wire.ProviderBesu, wantValid: true},
		{name: "unknown", provider: "reth", wantValid: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := goodConfig()
			c.Chains[0].Provider = tc.provider

			errs := Check(c)
			if tc.wantValid {
				require.Empty(t, errs)
				return
			}
			require.Contains(t, pathsOf(errs), "chains[0].provider")
		})
	}
}

func pathsOf(errs []wire.ValidationError) []string {
	out := make([]string, len(errs))
	for i, e := range errs {
		out[i] = e.Path
	}
	return out
}

func TestCheck_Invalid(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(c *wire.ConfigYAML)
		wantPath string
	}{
		{
			name:     "route source names no chain",
			mutate:   func(c *wire.ConfigYAML) { c.Relayer.Routes[0].Source = "99999" },
			wantPath: "relayer.routes[0].source",
		},
		{
			name:     "route destination names no chain",
			mutate:   func(c *wire.ConfigYAML) { c.Relayer.Routes[0].Destination = "99999" },
			wantPath: "relayer.routes[0].destination",
		},
		{
			name: "duplicate route id",
			mutate: func(c *wire.ConfigYAML) {
				c.Relayer.Routes = append(c.Relayer.Routes, c.Relayer.Routes[0])
			},
			wantPath: "relayer.routes[1].id",
		},
		{
			name:     "empty db path",
			mutate:   func(c *wire.ConfigYAML) { c.DB.URL = "" },
			wantPath: "db.url",
		},
		{
			name:     "unsupported in-memory db path",
			mutate:   func(c *wire.ConfigYAML) { c.DB.URL = ":memory:" },
			wantPath: "db.url",
		},
		{
			name:     "empty rpc url",
			mutate:   func(c *wire.ConfigYAML) { c.Chains[0].RPC.URL = "" },
			wantPath: "chains[0].rpc.url",
		},
		{
			name:     "duplicate chain id",
			mutate:   func(c *wire.ConfigYAML) { c.Chains[1].ID = "31337" },
			wantPath: "chains[1].id",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := goodConfig()
			tc.mutate(c)
			errs := Check(c)
			require.NotEmpty(t, errs, "mutation must produce a structural error")
			require.Contains(t, pathsOf(errs), tc.wantPath)
		})
	}
}
