package setup_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/e2e/e2etest"
	"github.com/cosmos/ibc/link/harness/ibclink"
	"github.com/cosmos/ibc/link/harness/ibclink/wire"
	"github.com/cosmos/ibc/link/harness/topology"
)

func TestValidateConfig_SelectedTopology(t *testing.T) {
	topo := e2etest.SelectedTopology(t)
	h := e2etest.StartHarness(t, topo)
	link := h.IBCLink()

	chainA := chainSpec(t, topo, topology.ChainA)

	// Only the chain's RPC URL is needed here — a neutral chain.Chain property, no EVM view required.
	a, err := h.Chains().Get(topology.ChainA)
	require.NoError(t, err)

	t.Run("live_ok", func(t *testing.T) {
		ctx := t.Context()
		res, err := link.ValidateConfig(ctx, true)
		require.NoError(t, err)
		require.NotNil(t, res)
		require.True(t, res.Valid)
		require.Empty(t, res.Errors)
		require.Contains(t, res.ResolvedChains, topology.ChainA)
		require.Contains(t, res.ResolvedChains, topology.ChainB)
	})

	t.Run("invalid_config_exit64", func(t *testing.T) {
		ctx := t.Context()
		bad := wire.ConfigYAML{
			Chains: []wire.Chain{
				{
					ID:       topology.ChainA,
					Type:     wire.ChainTypeEVM,
					Provider: chainA.Provider,
					ChainID:  chainA.ChainID,
					RPC:      wire.RPC{URL: a.RPCURL()},
				},
			},
			DB: wire.DB{Type: wire.DBTypeSQLite, URL: filepath.Join(t.TempDir(), "bad.db")},
			Relayer: wire.Relayer{
				Routes: []wire.Route{
					{ID: "route-bad", Source: topology.ChainA, Destination: "999999", Type: "evmToEvmAttested"},
				},
			},
		}
		runner := newRunner(t, writeConfig(t, bad))

		res, err := runner.ValidateConfig(ctx, false)
		requireExit(t, err, ibclink.ErrConfigInvalid, wire.ExitConfigInvalid)
		require.NotNil(t, res)
		require.False(t, res.Valid)
		require.True(t, hasErrorPath(res.Errors, "relayer.routes[0].destination"),
			"expected located validation error, got %+v", res.Errors)
	})

	t.Run("live_down_rpc_exit65", func(t *testing.T) {
		ctx := t.Context()
		down := wire.ConfigYAML{
			Chains: []wire.Chain{
				{
					ID:       topology.ChainA,
					Type:     wire.ChainTypeEVM,
					Provider: chainA.Provider,
					ChainID:  chainA.ChainID,
					RPC:      wire.RPC{URL: refusingRPCURL(t)},
				},
			},
			DB: wire.DB{Type: wire.DBTypeSQLite, URL: filepath.Join(t.TempDir(), "down.db")},
		}
		runner := newRunner(t, writeConfig(t, down))

		res, err := runner.ValidateConfig(ctx, true)
		requireExit(t, err, ibclink.ErrRPCUnreachable, wire.ExitRPCUnreachable)
		require.NotNil(t, res)
		require.True(t, res.Valid)
		require.True(t, hasErrorPath(res.Errors, "chains[0].rpc.url"),
			"expected located RPC error, got %+v", res.Errors)
	})

	t.Run("live_wrong_chain_id_exit65", func(t *testing.T) {
		ctx := t.Context()
		wrongID := wire.ConfigYAML{
			Chains: []wire.Chain{
				{
					ID:       topology.ChainA,
					Type:     wire.ChainTypeEVM,
					Provider: chainA.Provider,
					ChainID:  chainA.ChainID + 99999,
					RPC:      wire.RPC{URL: a.RPCURL()},
				},
			},
			DB: wire.DB{Type: wire.DBTypeSQLite, URL: filepath.Join(t.TempDir(), "wrongid.db")},
		}
		runner := newRunner(t, writeConfig(t, wrongID))

		structural, err := runner.ValidateConfig(ctx, false)
		require.NoError(t, err)
		require.True(t, structural.Valid)

		res, err := runner.ValidateConfig(ctx, true)
		requireExit(t, err, ibclink.ErrRPCUnreachable, wire.ExitRPCUnreachable)
		require.NotNil(t, res)
		require.True(t, res.Valid)
		require.True(t, hasErrorPath(res.Errors, "chains[0].chainId"),
			"expected located chain-id error, got %+v", res.Errors)
	})
}

func chainSpec(t *testing.T, topo topology.Topology, chainID string) wire.Chain {
	t.Helper()
	for _, spec := range topo.Chains {
		if spec.Chain.ID == chainID {
			return spec.Chain
		}
	}
	t.Fatalf("topology %s has no chain %s", topo.Name, chainID)
	return wire.Chain{}
}
