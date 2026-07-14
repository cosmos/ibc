package stub

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/cmd/configcmd"
)

const (
	relayerSignerAlias      = "relayer"
	missingSignerAlias      = "missing"
	firstChainEVMSignerPath = "chains[0].evmSigner"
	expectedDBURLPath       = "db.url"
)

func goodConfig() *configcmd.Config {
	return &configcmd.Config{
		Signers: []configcmd.Signer{{
			Alias: relayerSignerAlias, Type: configcmd.SignerTypeLocal, File: "/keys/relayer.json",
		}},
		Chains: []configcmd.Chain{
			{
				ID:        "31337",
				Type:      configcmd.ChainTypeEVM,
				ChainID:   31337,
				EVMSigner: relayerSignerAlias,
				RPC:       configcmd.RPC{URL: "http://127.0.0.1:8545"},
			},
			{
				ID:        "31338",
				Type:      configcmd.ChainTypeEVM,
				ChainID:   31338,
				EVMSigner: relayerSignerAlias,
				RPC:       configcmd.RPC{URL: "http://127.0.0.1:8546"},
			},
		},
		DB: configcmd.DB{Type: configcmd.DBTypeSQLite, URL: "/tmp/relayer.db"},
		Relayer: configcmd.Relayer{
			Routes: []configcmd.Route{
				{ID: "route-a-to-b", Source: "31337", Destination: "31338", Type: "evmToEvmAttested"},
			},
		},
	}
}

func TestCheck_Valid(t *testing.T) {
	require.Empty(t, checkConfig(goodConfig()), "a well-formed config has no structural errors")
}

func pathsOf(errs []configcmd.ValidationError) []string {
	out := make([]string, len(errs))
	for i, e := range errs {
		out[i] = e.Path
	}
	return out
}

func TestCheck_Invalid(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(c *configcmd.Config)
		wantPath string
	}{
		{
			name:     "route source names no chain",
			mutate:   func(c *configcmd.Config) { c.Relayer.Routes[0].Source = "99999" },
			wantPath: "relayer.routes[0].source",
		},
		{
			name:     "route destination names no chain",
			mutate:   func(c *configcmd.Config) { c.Relayer.Routes[0].Destination = "99999" },
			wantPath: "relayer.routes[0].destination",
		},
		{
			name: "duplicate route id",
			mutate: func(c *configcmd.Config) {
				c.Relayer.Routes = append(c.Relayer.Routes, c.Relayer.Routes[0])
			},
			wantPath: "relayer.routes[1].id",
		},
		{
			name:     "empty db path",
			mutate:   func(c *configcmd.Config) { c.DB.URL = "" },
			wantPath: expectedDBURLPath,
		},
		{
			name:     "unsupported in-memory db path",
			mutate:   func(c *configcmd.Config) { c.DB.URL = ":memory:" },
			wantPath: expectedDBURLPath,
		},
		{
			name:     "empty rpc url",
			mutate:   func(c *configcmd.Config) { c.Chains[0].RPC.URL = "" },
			wantPath: "chains[0].rpc.url",
		},
		{
			name:     "duplicate chain id",
			mutate:   func(c *configcmd.Config) { c.Chains[1].ID = "31337" },
			wantPath: "chains[1].id",
		},
		{
			name:     "route endpoint missing EVM relay signer",
			mutate:   func(c *configcmd.Config) { c.Chains[0].EVMSigner = "" },
			wantPath: firstChainEVMSignerPath,
		},
		{
			name:     "route endpoint names unknown EVM relay signer",
			mutate:   func(c *configcmd.Config) { c.Chains[0].EVMSigner = missingSignerAlias },
			wantPath: firstChainEVMSignerPath,
		},
		{
			name: "non-route chain names unknown EVM relay signer",
			mutate: func(c *configcmd.Config) {
				c.Relayer.Routes = nil
				c.Chains[0].EVMSigner = missingSignerAlias
			},
			wantPath: firstChainEVMSignerPath,
		},
		{
			name:     "test-app deployment names unknown signer",
			mutate:   func(c *configcmd.Config) { c.Chains[0].TestAppSigner = missingSignerAlias },
			wantPath: "chains[0].testAppSigner",
		},
		{
			name:     "duplicate signer alias",
			mutate:   func(c *configcmd.Config) { c.Signers = append(c.Signers, c.Signers[0]) },
			wantPath: "signers[1].alias",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := goodConfig()
			tc.mutate(c)
			errs := checkConfig(c)
			require.NotEmpty(t, errs, "mutation must produce a structural error")
			require.Contains(t, pathsOf(errs), tc.wantPath)
		})
	}
}

func TestLiveValidationChecksReportedChainID(t *testing.T) {
	rpc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":"0x7a69"}`)
	}))
	t.Cleanup(rpc.Close)

	config := &configcmd.Config{Chains: []configcmd.Chain{{
		ID: testChainA, Type: configcmd.ChainTypeEVM, ChainID: 1, RPC: configcmd.RPC{URL: rpc.URL},
	}}}
	errs := pingChains(t.Context(), config)
	require.Len(t, errs, 1)
	require.Equal(t, "chains[0].chainId", errs[0].Path)
	require.Contains(t, errs[0].Msg, "node reports 31337")
}

func TestIndentedJSONReturnsStdoutWriteFailure(t *testing.T) {
	err := printIndentedJSON(failingWriter{}, configcmd.ValidateResult{Valid: true})
	require.ErrorContains(t, err, "write JSON")
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, fmt.Errorf("stdout closed") }
