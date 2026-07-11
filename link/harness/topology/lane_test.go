package topology

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"
)

func TestLaneGrid(t *testing.T) {
	lanes := map[string]Lane{
		"anvil":          Anvil,
		"anvil-interval": AnvilInterval,
		"besu":           Besu,
	}
	shapes := map[string]Shape{
		"two-evm": TwoEVM(),
	}
	routeTypes := map[string][2]string{
		"two-evm": {wire.RouteEVMToEVMAttested, wire.RouteEVMToEVMAttested},
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

func TestLane_ZeroShapePanics(t *testing.T) {
	for _, lane := range []Lane{Anvil, AnvilInterval, Besu} {
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

}
