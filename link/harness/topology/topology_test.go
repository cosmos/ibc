package topology

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"
)

func TestWithManualRelay(t *testing.T) {
	base := Anvil(TwoEVM())
	manual := base.WithManualRelay(RouteAtoB)

	flipped, ok := manual.Config.Route(RouteAtoB)
	require.True(t, ok)
	require.False(t, flipped.AutoRelayEnabled())
	kept, ok := manual.Config.Route(RouteBtoA)
	require.True(t, ok)
	require.True(t, kept.AutoRelayEnabled())
	require.Equal(t, base.Name+"+manual", manual.Name)

	orig, ok := base.Config.Route(RouteAtoB)
	require.True(t, ok)
	require.True(t, orig.AutoRelayEnabled())

	for _, r := range base.WithManualRelay().Config.Relayer.Routes {
		require.False(t, r.AutoRelayEnabled(), "route %s must be manual when no ids are named", r.ID)
	}

	require.Panics(t, func() { base.WithManualRelay("no-such-route") })
}

func TestValidate(t *testing.T) {
	evm := func(id string, chainID uint64) ChainSpec {
		return ChainSpec{
			Chain:     wire.Chain{ID: id, Type: wire.ChainTypeEVM, Provider: wire.ProviderAnvil, ChainID: chainID},
			Provision: Provision{Mode: ProvisionManaged, Launcher: LauncherAnvil},
		}
	}
	topo := func(chains []ChainSpec, routes ...wire.Route) Topology {
		return Topology{
			Name:   "validate-under-test",
			Chains: chains,
			Config: wire.ConfigYAML{Relayer: wire.Relayer{Routes: routes}},
		}
	}
	route := func(id, src, dst, typ string) wire.Route {
		return wire.Route{ID: id, Source: src, Destination: dst, Type: typ}
	}

	cases := []struct {
		name    string
		topo    Topology
		wantErr string
	}{
		{
			name: "valid three-chain fan-in",
			topo: topo(
				[]ChainSpec{evm("a", 31637), evm("b", 31638), evm("c", 31639)},
				route("b-to-a", "b", "a", wire.RouteEVMToEVMAttested),
				route("c-to-a", "c", "a", wire.RouteEVMToEVMAttested),
			),
		},
		{
			name:    "empty chain id",
			topo:    topo([]ChainSpec{evm("", 31337)}),
			wantErr: "chain with empty id",
		},
		{
			name:    "duplicate chain id",
			topo:    topo([]ChainSpec{evm("a", 31337), evm("a", 31338)}),
			wantErr: "duplicate chain id a",
		},
		{
			name: "unknown chain family",
			topo: topo([]ChainSpec{{
				Chain: wire.Chain{ID: "a", Type: "solana"},
			}}),
			wantErr: `unknown family "solana"`,
		},
		{
			name:    "shared numeric EVM chain id",
			topo:    topo([]ChainSpec{evm("a", 31337), evm("b", 31337)}),
			wantErr: "share EVM chain id 31337",
		},
		{
			name: "empty route id",
			topo: topo(
				[]ChainSpec{evm("a", 31337), evm("b", 31338)},
				route("", "a", "b", wire.RouteEVMToEVMAttested),
			),
			wantErr: "route with empty id",
		},
		{
			name: "duplicate route id",
			topo: topo(
				[]ChainSpec{evm("a", 31337), evm("b", 31338)},
				route("r", "a", "b", wire.RouteEVMToEVMAttested),
				route("r", "b", "a", wire.RouteEVMToEVMAttested),
			),
			wantErr: "duplicate route id r",
		},
		{
			name: "unknown source chain",
			topo: topo(
				[]ChainSpec{evm("a", 31337)},
				route("r", "ghost", "a", wire.RouteEVMToEVMAttested),
			),
			wantErr: `unknown source chain "ghost"`,
		},
		{
			name: "unknown destination chain",
			topo: topo(
				[]ChainSpec{evm("a", 31337)},
				route("r", "a", "ghost", wire.RouteEVMToEVMAttested),
			),
			wantErr: `unknown destination chain "ghost"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.topo.Validate()
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
			require.ErrorContains(t, err, "topology validate-under-test")
		})
	}
}
