package topology

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"
	"github.com/cosmos/ibc/link/harness/testkeys"
)

// update regenerates the committed golden YAML instead of asserting against it:
//
//	go test ./harness/topology -run TestCompile_Golden -update
var update = flag.Bool("update", false, "update golden files")

const (
	dbPath = "/run/ibc-link/relayer.db"
	rpcA   = "http://127.0.0.1:8545"
	rpcB   = "http://127.0.0.1:8546"
	rpcExt = "http://198.51.100.7:8545"
)

func fixtureTopology() Topology {
	return Topology{
		Name: "evm-evm-attested",
		Chains: []ChainSpec{
			{
				Chain: wire.Chain{
					ID:           ChainA,
					Type:         wire.ChainTypeEVM,
					Provider:     wire.ProviderAnvil,
					ChainID:      31337,
					EVMSignerKey: testkeys.RelayerPrivateKeyHex,
				},
				Provision: Provision{Mode: ProvisionManaged, Launcher: LauncherAnvil},
			},
			{
				Chain: wire.Chain{
					ID:           ChainB,
					Type:         wire.ChainTypeEVM,
					Provider:     wire.ProviderAnvil,
					ChainID:      31338,
					EVMSignerKey: testkeys.RelayerPrivateKeyHex,
				},
				Provision: Provision{Mode: ProvisionManaged, Launcher: LauncherAnvil},
			},
		},
		Config: wire.ConfigYAML{
			Relayer: wire.Relayer{
				Routes: []wire.Route{
					{ID: RouteAtoB, Source: ChainA, Destination: ChainB, Type: wire.RouteEVMToEVMAttested},
				},
			},
		},
	}
}

func fixtureBindings() RuntimeBindings {
	return RuntimeBindings{
		ChainRPC: map[string]string{ChainA: rpcA, ChainB: rpcB},
		DBPath:   dbPath,
	}
}

func TestCompile_Golden(t *testing.T) {
	cfg, err := Compile(fixtureTopology(), fixtureBindings())
	require.NoError(t, err)

	got, err := cfg.Marshal()
	require.NoError(t, err)

	goldenPath := filepath.Join("testdata", "evm-evm-attested.golden.yaml")
	if *update {
		require.NoError(t, os.MkdirAll("testdata", 0o755))
		require.NoError(t, os.WriteFile(goldenPath, got, 0o644))
		t.Logf("updated golden %s", goldenPath)
	}

	want, err := os.ReadFile(goldenPath)
	require.NoError(t, err, "missing golden; run with -update to generate")
	require.Equal(t, string(want), string(got), "compiled YAML diverged from golden; run with -update if intended")

	out := string(got)

	require.Contains(t, out, rpcA)
	require.Contains(t, out, rpcB)
	require.Contains(t, out, dbPath)

	require.Contains(t, out, "route-a-to-b")
}

func TestCompile_DoesNotMutateTopology(t *testing.T) {
	topo := fixtureTopology()

	cfg, err := Compile(topo, fixtureBindings())
	require.NoError(t, err)

	require.NotEmpty(t, cfg.Chains[0].RPC.URL)
	require.NotEmpty(t, cfg.DB.URL)

	require.Empty(t, topo.Chains[0].Chain.RPC.URL)
	require.Empty(t, topo.Config.DB.URL)

	cfg.Relayer.Routes[0].ID = "changed"
	require.Equal(t, RouteAtoB, topo.Config.Relayer.Routes[0].ID)
}

func TestCompile_Managed(t *testing.T) {
	t.Run("missing RPC binding errors", func(t *testing.T) {
		rb := fixtureBindings()
		delete(rb.ChainRPC, ChainB)
		_, err := Compile(fixtureTopology(), rb)
		require.ErrorIs(t, err, ErrMissingRPC)
	})

	t.Run("empty RPC binding errors", func(t *testing.T) {
		rb := fixtureBindings()
		rb.ChainRPC[ChainB] = ""
		_, err := Compile(fixtureTopology(), rb)
		require.ErrorIs(t, err, ErrMissingRPC)
	})
}

func externalTopology(externalURL string) Topology {
	return Topology{
		Name: "evm-evm-external",
		Chains: []ChainSpec{
			{
				Chain: wire.Chain{
					ID:       ChainA,
					Type:     wire.ChainTypeEVM,
					Provider: wire.ProviderAnvil,
					ChainID:  31337,
				},
				Provision: Provision{Mode: ProvisionManaged, Launcher: LauncherAnvil},
			},
			{
				Chain: wire.Chain{
					ID:       ChainB,
					Type:     wire.ChainTypeEVM,
					Provider: wire.ProviderAnvil,
					ChainID:  31338,
				},
				Provision: Provision{Mode: ProvisionExternal, RPCURL: externalURL},
			},
		},
		Config: wire.ConfigYAML{
			Relayer: wire.Relayer{
				Routes: []wire.Route{
					{ID: RouteAtoB, Source: ChainA, Destination: ChainB, Type: wire.RouteEVMToEVMAttested},
				},
			},
		},
	}
}

func TestCompile_External(t *testing.T) {
	t.Run("takes Provision.RPCURL verbatim", func(t *testing.T) {
		rb := RuntimeBindings{ChainRPC: map[string]string{ChainA: rpcA}, DBPath: dbPath}
		cfg, err := Compile(externalTopology(rpcExt), rb)
		require.NoError(t, err)

		require.Equal(t, rpcA, cfg.Chains[0].RPC.URL, "managed chain takes its binding")
		require.Equal(t, rpcExt, cfg.Chains[1].RPC.URL, "external chain takes its static URL")
	})

	t.Run("missing URL errors", func(t *testing.T) {
		rb := RuntimeBindings{ChainRPC: map[string]string{ChainA: rpcA}, DBPath: dbPath}
		_, err := Compile(externalTopology(""), rb)
		require.ErrorIs(t, err, ErrExternalRPCRequired)
	})

	t.Run("binding never overwrites the static URL", func(t *testing.T) {
		rb := RuntimeBindings{
			ChainRPC: map[string]string{ChainA: rpcA, ChainB: "http://10.0.0.1:9999"},
			DBPath:   dbPath,
		}
		cfg, err := Compile(externalTopology(rpcExt), rb)
		require.NoError(t, err)
		require.Equal(t, rpcExt, cfg.Chains[1].RPC.URL, "external URL must not be overwritten by a binding")
	})
}
