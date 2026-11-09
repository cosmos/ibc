// SPDX-License-Identifier: Apache-2.0

package yaml

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-yaml"
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

func TestMaskEnvPaths(t *testing.T) {
	for _, tt := range []struct {
		name       string
		file       string
		env        map[string]string
		keepValues []string
		change     func(t *testing.T, cfg *testConfig)
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
			name:       "keepValues",
			file:       "04",
			env:        map[string]string{"token": "secret-token", "router_a": "router-a-old", "router_b": "router-b-old"},
			keepValues: []string{"$.chains[*].router"},
			change: func(t *testing.T, cfg *testConfig) {
				require.Len(t, cfg.Chains, 3)
				require.Empty(t, cfg.Chains[2].Router, "router_c must be unset")

				// kept: new value wins
				cfg.Chains[0].Router = "router-a-new"

				// not kept: env reference wins
				cfg.Chains[0].RPC = "https://rpc.com/abc/changed"
			},
		},
		{
			name: "flowStyle",
			file: "05",
			env:  map[string]string{"token": "secret-token"},
			change: func(t *testing.T, cfg *testConfig) {
				require.Equal(t, []testChain{
					{ID: "a", RPC: "https://rpc.com/abc/secret-token", Auth: &testAuth{}},
				}, cfg.Chains)

				cfg.Chains[0].Router = "router-a"
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			cfg, paths := loadTestConfig(t, filepath.Join("testdata", tt.file+".in.yaml"))
			tt.change(t, &cfg)

			paths = slices.DeleteFunc(paths, func(p EnvPath) bool {
				return slices.ContainsFunc(tt.keepValues, func(pattern string) bool {
					return PathMatches(p.Path, pattern)
				})
			})

			expected, err := os.ReadFile(filepath.Join("testdata", tt.file+".out.yaml"))
			require.NoError(t, err)

			// ACT
			tree, err := MaskEnvPaths(cfg, paths)
			require.NoError(t, err)

			actual, err := yaml.Marshal(tree)
			require.NoError(t, err)

			// ASSERT
			require.Equal(t, stripLicenseHeader(string(expected)), string(actual))
		})
	}
}

func TestFindEnvPaths(t *testing.T) {
	for _, tt := range []struct {
		name     string
		raw      string
		expected []EnvPath
	}{
		{name: "empty", raw: "", expected: []EnvPath{}},
		{name: "noEnv", raw: "a: $\nb: price$\n", expected: []EnvPath{}},
		{
			name: "flowStyle",
			raw:  "tls: {caFile: ${CA}}\nlist: [x, $B]\n",
			expected: []EnvPath{
				{Path: "$.tls.caFile", Template: "${CA}"},
				{Path: "$.list[1]", Template: "$B"},
			},
		},
		{
			name: "alias",
			raw:  "db: &db postgres://u:${PASS}@h\nserver:\n  addr: *db\n",
			expected: []EnvPath{
				{Path: "$.db", Template: "postgres://u:${PASS}@h"},
				{Path: "$.server.addr", Template: "postgres://u:${PASS}@h"},
			},
		},
		{
			name: "quoted",
			raw:  "url: \"${DB} #1\"\n",
			expected: []EnvPath{
				{Path: "$.url", Template: "${DB} #1", quote: '"'},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// ACT
			paths, err := FindEnvPaths([]byte(tt.raw))

			// ASSERT
			require.NoError(t, err)
			require.Equal(t, tt.expected, paths)
		})
	}
}

// loadTestConfig mirrors config.LoadFromFile: find env paths in the raw bytes, decode the expanded bytes.
func loadTestConfig(t *testing.T, path string) (testConfig, []EnvPath) {
	t.Helper()

	bz, err := os.ReadFile(path)
	require.NoError(t, err)

	paths, err := FindEnvPaths(bz)
	require.NoError(t, err)

	var cfg testConfig
	require.NoError(t, yaml.UnmarshalWithOptions([]byte(os.ExpandEnv(string(bz))), &cfg, yaml.DisallowUnknownField()))

	return cfg, paths
}

func stripLicenseHeader(s string) string {
	return strings.TrimPrefix(s, "# SPDX-License-Identifier: Apache-2.0\n\n")
}
