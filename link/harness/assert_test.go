package harness

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"
	"github.com/cosmos/ibc/link/harness/topology"
)

func twoChainTopology() topology.Topology {
	return topology.Topology{
		Name: "test",
		Chains: []topology.ChainSpec{
			{Chain: wire.Chain{ID: "chain-a"}},
			{Chain: wire.Chain{ID: "chain-b"}},
		},
	}
}

func TestAssertChainsConnected(t *testing.T) {
	topo := twoChainTopology()

	ready := wire.Readiness{ChainsConnected: []string{"chain-a", "chain-b"}}
	require.NoError(t, assertChainsConnected(ready, topo))

	partial := wire.Readiness{ChainsConnected: []string{"chain-a"}}
	err := assertChainsConnected(partial, topo)
	require.ErrorContains(t, err, "did not connect to chain chain-b")
}

func TestAssertDeploymentMatchesTopology(t *testing.T) {
	topo := twoChainTopology()

	full := &wire.Deployment{Chains: map[string]wire.ChainDeployment{"chain-a": {}, "chain-b": {}}}
	require.NoError(t, assertDeploymentMatchesTopology(full, topo))

	partial := &wire.Deployment{Chains: map[string]wire.ChainDeployment{"chain-a": {}}}
	err := assertDeploymentMatchesTopology(partial, topo)
	require.ErrorContains(t, err, "must report chain chain-b")
}

func TestTopologySummaryNamesExternalLogGap(t *testing.T) {
	topo := topology.Topology{
		Name: "mixed",
		Chains: []topology.ChainSpec{
			{
				Chain:     wire.Chain{ID: "chain-a", ChainID: 31337},
				Provision: topology.Provision{Mode: topology.ProvisionManaged, Launcher: topology.LauncherAnvil},
			},
			{
				Chain:     wire.Chain{ID: "chain-b", ChainID: 1},
				Provision: topology.Provision{Mode: topology.ProvisionExternal, RPCURL: "http://10.0.0.1:8545"},
			},
		},
		Config: wire.ConfigYAML{Relayer: wire.Relayer{Routes: []wire.Route{
			{ID: "r", Source: "chain-a", Destination: "chain-b", Type: wire.RouteEVMToEVMAttested},
		}}},
	}
	// The doctrine under test (AGENTS.md: named gaps, never silent omissions): an external chain's
	// uncollectable logs are called out in the summary — and only there, so the managed chain's line
	// carries no such marker. Wording beyond the marker is not contract.
	summary := topologySummary(topo)
	require.Equal(t, 1, strings.Count(summary, "(logs unavailable)"),
		"exactly the external chain is gap-marked")
}
