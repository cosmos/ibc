package topology

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"
)

// TestLaneGrid binds every lane to every shape and locks the invariants shared by all cells:
// <shape>-<lane> names, Validate-clean output, canonical slot/route ids, and route types derived
// from the slot families.
func TestLaneGrid(t *testing.T) {
	lanes := map[string]Lane{
		"anvil":          Anvil,
		"anvil-interval": AnvilInterval,
		"besu":           Besu,
		"sandbox":        Sandbox,
	}
	shapes := map[string]Shape{
		"two-evm":    TwoEVM(),
		"evm-cosmos": EVMCosmos(),
	}
	routeTypes := map[string][2]string{
		"two-evm":    {wire.RouteEVMToEVMAttested, wire.RouteEVMToEVMAttested},
		"evm-cosmos": {wire.RouteEVMToCosmosAttested, wire.RouteCosmosToEVMAttested},
	}

	for laneName, lane := range lanes {
		for shapeName, shape := range shapes {
			topo := lane(shape)
			t.Run(topo.Name, func(t *testing.T) {
				require.Equal(t, shapeName+"-"+laneName, topo.Name)
				require.NoError(t, topo.Validate())

				require.Len(t, topo.Chains, 2)
				require.Equal(t, ChainA, topo.Chains[0].Chain.ID)
				require.Equal(t, ChainB, topo.Chains[1].Chain.ID)
				for _, spec := range topo.Chains {
					require.Equal(t, ProvisionManaged, spec.Provision.Mode)
				}

				aToB, ok := topo.Config.Route(RouteAtoB)
				require.True(t, ok)
				require.Equal(t, routeTypes[shapeName][0], aToB.Type)
				require.True(t, aToB.AutoRelayEnabled())
				bToA, ok := topo.Config.Route(RouteBtoA)
				require.True(t, ok)
				require.Equal(t, routeTypes[shapeName][1], bToA.Type)
				require.True(t, bToA.AutoRelayEnabled())
			})
		}
	}
}

// Every lane must funnel the zero Shape into bind's guard — a lane bypassing bind would lose it.
func TestLane_ZeroShapePanics(t *testing.T) {
	for _, lane := range []Lane{Anvil, AnvilInterval, Besu, Sandbox} {
		require.Panics(t, func() { lane(Shape{}) })
	}
}

func TestLaneSlotBindings(t *testing.T) {
	launcher := func(topo Topology, i int) string { return topo.Chains[i].Provision.Launcher }

	t.Run("anvil binds instant anvil to every EVM slot", func(t *testing.T) {
		topo := Anvil(TwoEVM())
		for i, spec := range topo.Chains {
			require.Equal(t, LauncherAnvil, launcher(topo, i))
			require.Equal(t, uint64(31337+i), spec.Chain.ChainID)
			require.Zero(t, spec.Timing, "instant slots resolve timing from the launcher default")
		}
	})

	t.Run("anvil-interval sets the block-cadence timing on every EVM slot", func(t *testing.T) {
		topo := AnvilInterval(TwoEVM())
		for i, spec := range topo.Chains {
			require.Equal(t, LauncherAnvil, launcher(topo, i))
			require.Equal(t, uint64(31437+i), spec.Chain.ChainID)
			require.Equal(t, 2*time.Second, spec.Timing.BlockInterval)
		}
	})

	t.Run("besu binds dockerized besu to every EVM slot", func(t *testing.T) {
		topo := Besu(TwoEVM())
		for i, spec := range topo.Chains {
			require.Equal(t, LauncherBesu, launcher(topo, i))
			require.Equal(t, uint64(32337+i), spec.Chain.ChainID)
		}
	})

	t.Run("sandbox lane is heterogeneous: anvil slot A, EVM-presented sandboxd slot B", func(t *testing.T) {
		topo := Sandbox(TwoEVM())
		require.Equal(t, LauncherAnvil, launcher(topo, 0))
		require.Equal(t, LauncherSandbox, launcher(topo, 1))
		b := topo.Chains[1].Chain
		require.Equal(t, wire.ChainTypeEVM, b.Type)
		require.Equal(t, wire.ProviderSandbox, b.Provider)
		require.Equal(t, uint64(19460), b.ChainID)
		require.Empty(t, b.CosmosChainID)
	})

	t.Run("cosmos slots bind to sandboxd in every lane", func(t *testing.T) {
		for _, lane := range []Lane{Anvil, AnvilInterval, Besu, Sandbox} {
			topo := lane(EVMCosmos())
			b := topo.Chains[1]
			require.Equal(t, LauncherSandbox, b.Provision.Launcher, "lane %s", topo.Name)
			require.Equal(t, wire.ChainTypeCosmos, b.Chain.Type)
			require.Equal(t, "sandbox-cosmos-1", b.Chain.CosmosChainID)
			require.Zero(t, b.Chain.ChainID, "a cosmos slot carries no numeric EVM chain id")
			require.Zero(t, b.Timing, "cosmos slots resolve timing from the sandbox launcher default")
		}
	})
}
