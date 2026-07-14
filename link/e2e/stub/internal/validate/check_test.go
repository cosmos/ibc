package validate

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"
)

const (
	relayerSignerAlias      = "relayer"
	missingSignerAlias      = "missing"
	firstChainEVMSignerPath = "chains[0].evmSigner"
	expectedDBURLPath       = "db.url"
)

func goodConfig() *wire.ConfigYAML {
	return &wire.ConfigYAML{
		Signers: []wire.Signer{{
			Alias: relayerSignerAlias, Type: wire.SignerTypeLocal, File: "/keys/relayer.json",
		}},
		Chains: []wire.Chain{
			{
				ID:        "31337",
				Type:      "evm",
				ChainID:   31337,
				EVMSigner: relayerSignerAlias,
				RPC:       wire.RPC{URL: "http://127.0.0.1:8545"},
			},
			{
				ID:        "31338",
				Type:      "evm",
				ChainID:   31338,
				EVMSigner: relayerSignerAlias,
				RPC:       wire.RPC{URL: "http://127.0.0.1:8546"},
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
			wantPath: expectedDBURLPath,
		},
		{
			name:     "unsupported in-memory db path",
			mutate:   func(c *wire.ConfigYAML) { c.DB.URL = ":memory:" },
			wantPath: expectedDBURLPath,
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
		{
			name:     "route endpoint missing EVM relay signer",
			mutate:   func(c *wire.ConfigYAML) { c.Chains[0].EVMSigner = "" },
			wantPath: firstChainEVMSignerPath,
		},
		{
			name:     "route endpoint names unknown EVM relay signer",
			mutate:   func(c *wire.ConfigYAML) { c.Chains[0].EVMSigner = missingSignerAlias },
			wantPath: firstChainEVMSignerPath,
		},
		{
			name: "non-route chain names unknown EVM relay signer",
			mutate: func(c *wire.ConfigYAML) {
				c.Relayer.Routes = nil
				c.Chains[0].EVMSigner = missingSignerAlias
			},
			wantPath: firstChainEVMSignerPath,
		},
		{
			name:     "test-app deployment names unknown signer",
			mutate:   func(c *wire.ConfigYAML) { c.Chains[0].TestAppSigner = missingSignerAlias },
			wantPath: "chains[0].testAppSigner",
		},
		{
			name:     "duplicate signer alias",
			mutate:   func(c *wire.ConfigYAML) { c.Signers = append(c.Signers, c.Signers[0]) },
			wantPath: "signers[1].alias",
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
