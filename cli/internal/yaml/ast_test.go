// SPDX-License-Identifier: Apache-2.0

package yaml

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
	"github.com/stretchr/testify/require"
)

type testConfig struct {
	Host       string        `yaml:"host"`
	DB         string        `yaml:"db"`
	MaxTimeout time.Duration `yaml:"max_timeout"`
	Debug      bool          `yaml:"debug,omitempty"`
	RPC        []testRPC     `yaml:"rpc,omitempty"`
	Chains     []testChain   `yaml:"chains,omitempty"`
}

type testRPC struct {
	ChainID   string `yaml:"chainId"`
	URL       string `yaml:"url"`
	BatchSize int    `yaml:"batchSize"`
	MaxPeers  uint   `yaml:"maxPeers"`
	Archive   bool   `yaml:"archive"`
}

type testChain struct {
	ID     string    `yaml:"id"`
	RPC    string    `yaml:"rpc"`
	WS     string    `yaml:"ws,omitempty"`
	Router string    `yaml:"router,omitempty"`
	Auth   *testAuth `yaml:"auth,omitempty"`
}

type testAuth struct {
	SecretKey string `yaml:"secretKey,omitempty"`
}

func TestMerge(t *testing.T) {
	for _, tt := range []struct {
		name             string
		file             string
		env              map[string]string
		forceUpdatePaths []string
		change           func(t *testing.T, cfg *testConfig)
	}{
		{
			name: "scalars",
			file: "01",
			env: map[string]string{
				"port":        "9090",
				"db_conn":     "postgres://user:secret@localhost/db",
				"max_timeout": "5s",
			},
			change: func(t *testing.T, cfg *testConfig) {
				require.Equal(t, "127.0.0.1:9090", cfg.Host)
				require.Equal(t, 5*time.Second, cfg.MaxTimeout)

				// env-backed values win over changes
				cfg.Host = "0.0.0.0:1"
				cfg.Debug = true
			},
		},
		{
			name: "sequenceOfRPCs",
			file: "02",
			env: map[string]string{
				"token":        "secret-token",
				"fallback_rpc": "",
				"batch_size":   "25",
				"max_peers":    "3",
				"archive":      "true",
			},
			change: func(t *testing.T, cfg *testConfig) {
				require.Equal(t, []testRPC{
					{ChainID: "a", URL: "https://rpc.com/abc/secret-token", BatchSize: 25, MaxPeers: 3, Archive: true},
					{ChainID: "b", URL: "https://plain.example", BatchSize: 10, MaxPeers: 1},
					{ChainID: "c", URL: "", BatchSize: 10, MaxPeers: 1},
				}, cfg.RPC)

				cfg.RPC[0].BatchSize = 50
				cfg.RPC[1].URL = "https://changed.example"
				cfg.RPC[1].BatchSize = 20
				cfg.RPC[1].Archive = true
				cfg.RPC = append(cfg.RPC, testRPC{ChainID: "d", URL: "https://new.example", BatchSize: 5, MaxPeers: 2})
			},
		},
		{
			name: "sequenceOfMappings",
			file: "03",
			env:  map[string]string{"port": "9090", "token": "secret-token", "ws": ""},
			change: func(t *testing.T, cfg *testConfig) {
				require.Len(t, cfg.Chains, 2)
				require.Equal(t, "https://rpc.com/abc/secret-token", cfg.Chains[0].RPC)
				require.Empty(t, cfg.Chains[0].WS)
				require.Equal(t, &testAuth{}, cfg.Chains[0].Auth, "my_secret must be unset")
				require.Equal(t, "127.0.0.1:9090", cfg.Chains[1].RPC)

				cfg.Chains[1].Router = "router-b"
				cfg.Chains = append(cfg.Chains, testChain{ID: "c", RPC: "https://c.example"})
			},
		},
		{
			name:             "forceUpdatePaths",
			file:             "04",
			env:              map[string]string{"token": "secret-token", "router_a": "router-a-old", "router_b": "router-b-old"},
			forceUpdatePaths: []string{"$.chains[*].router"},
			change: func(t *testing.T, cfg *testConfig) {
				require.Len(t, cfg.Chains, 3)
				require.Empty(t, cfg.Chains[2].Router, "router_c must be unset")

				// forced: new value wins
				cfg.Chains[0].Router = "router-a-new"

				// not forced: env reference wins
				cfg.Chains[0].RPC = "https://rpc.com/abc/changed"
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			cfg, original := loadTestConfig(t, filepath.Join("testdata", tt.file+".in.yaml"))
			tt.change(t, &cfg)

			expected, err := os.ReadFile(filepath.Join("testdata", tt.file+".out.yaml"))
			require.NoError(t, err)

			// ACT
			out := filepath.Join(t.TempDir(), "out.yaml")
			writeTestConfig(t, cfg, original, out, PreserveEnv(tt.forceUpdatePaths...))

			// ASSERT
			actual, err := os.ReadFile(out)
			require.NoError(t, err)
			require.Equal(t, stripLicenseHeader(string(expected)), string(actual))
		})
	}
}

// loadTestConfig mirrors config.LoadFromFile: keep the raw AST, decode the env-expanded bytes.
func loadTestConfig(t *testing.T, path string) (testConfig, *ast.File) {
	t.Helper()

	bz, err := os.ReadFile(path)
	require.NoError(t, err)

	original, err := parser.ParseBytes(bz, parser.ParseComments)
	require.NoError(t, err)

	var cfg testConfig
	require.NoError(t, yaml.UnmarshalWithOptions([]byte(os.ExpandEnv(string(bz))), &cfg, yaml.DisallowUnknownField()))

	return cfg, original
}

func writeTestConfig(t *testing.T, cfg testConfig, original *ast.File, path string, condition MergeCondition) {
	t.Helper()

	file, err := ValueToFile(cfg)
	require.NoError(t, err)

	merged, err := Merge(file, original, condition)
	require.NoError(t, err)

	bz, err := yaml.Marshal(merged.Docs[0].Body)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(path, bz, 0o600))
}

func stripLicenseHeader(s string) string {
	return strings.TrimPrefix(s, "# SPDX-License-Identifier: Apache-2.0\n\n")
}
