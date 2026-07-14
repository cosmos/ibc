package setup_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/e2e/e2etest"
	"github.com/cosmos/ibc/link/e2e/internal/synthetic"
	"github.com/cosmos/ibc/link/harness/ibclink"
	"github.com/cosmos/ibc/link/harness/ibclink/wire"
)

func TestValidateConfig_SelectedSuite(t *testing.T) {
	env := e2etest.Start(t, e2etest.SelectedSuite(t))
	a, err := env.Chain(e2etest.ChainA)
	require.NoError(t, err)
	b, err := env.Chain(e2etest.ChainB)
	require.NoError(t, err)
	relayerSigner := writeLocalSigner(t, "relayer")

	valid := wire.ConfigYAML{
		Signers: []wire.Signer{relayerSigner},
		Chains: []wire.Chain{
			{
				ID: string(e2etest.ChainA), Type: wire.ChainTypeEVM, ChainID: a.EVMChainID(),
				EVMSigner: relayerSigner.Alias, RPC: wire.RPC{URL: a.RPCURL()},
			},
			{
				ID: string(e2etest.ChainB), Type: wire.ChainTypeEVM, ChainID: b.EVMChainID(),
				EVMSigner: relayerSigner.Alias, RPC: wire.RPC{URL: b.RPCURL()},
			},
		},
		DB: wire.DB{Type: wire.DBTypeSQLite, URL: filepath.Join(t.TempDir(), "valid.db")},
		Relayer: wire.Relayer{Routes: []wire.Route{{
			ID: string(synthetic.RouteAtoB), Source: string(e2etest.ChainA), Destination: string(e2etest.ChainB),
			Type: wire.RouteEVMToEVMAttested,
		}}},
	}
	driver := newDriver(t, writeConfig(t, valid))

	t.Run("live_ok", func(t *testing.T) {
		ctx := t.Context()
		res, err := driver.ValidateConfig(ctx, true)
		require.NoError(t, err)
		require.NotNil(t, res)
		require.True(t, res.Valid)
		require.Empty(t, res.Errors)
		require.Contains(t, res.ResolvedChains, string(e2etest.ChainA))
		require.Contains(t, res.ResolvedChains, string(e2etest.ChainB))
	})

	t.Run("invalid_config_exit64", func(t *testing.T) {
		ctx := t.Context()
		bad := wire.ConfigYAML{
			Chains: []wire.Chain{
				{
					ID:      string(e2etest.ChainA),
					Type:    wire.ChainTypeEVM,
					ChainID: a.EVMChainID(),
					RPC:     wire.RPC{URL: a.RPCURL()},
				},
			},
			DB: wire.DB{Type: wire.DBTypeSQLite, URL: filepath.Join(t.TempDir(), "bad.db")},
			Relayer: wire.Relayer{
				Routes: []wire.Route{
					{
						ID:          "route-bad",
						Source:      string(e2etest.ChainA),
						Destination: "999999",
						Type:        wire.RouteEVMToEVMAttested,
					},
				},
			},
		}
		driver := newDriver(t, writeConfig(t, bad))

		res, err := driver.ValidateConfig(ctx, false)
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
					ID:      string(e2etest.ChainA),
					Type:    wire.ChainTypeEVM,
					ChainID: a.EVMChainID(),
					RPC:     wire.RPC{URL: refusingRPCURL(t)},
				},
			},
			DB: wire.DB{Type: wire.DBTypeSQLite, URL: filepath.Join(t.TempDir(), "down.db")},
		}
		driver := newDriver(t, writeConfig(t, down))

		res, err := driver.ValidateConfig(ctx, true)
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
					ID:      string(e2etest.ChainA),
					Type:    wire.ChainTypeEVM,
					ChainID: a.EVMChainID() + 99999,
					RPC:     wire.RPC{URL: a.RPCURL()},
				},
			},
			DB: wire.DB{Type: wire.DBTypeSQLite, URL: filepath.Join(t.TempDir(), "wrongid.db")},
		}
		driver := newDriver(t, writeConfig(t, wrongID))

		structural, err := driver.ValidateConfig(ctx, false)
		require.NoError(t, err)
		require.True(t, structural.Valid)

		res, err := driver.ValidateConfig(ctx, true)
		requireExit(t, err, ibclink.ErrRPCUnreachable, wire.ExitRPCUnreachable)
		require.NotNil(t, res)
		require.True(t, res.Valid)
		require.True(t, hasErrorPath(res.Errors, "chains[0].chainId"),
			"expected located chain-id error, got %+v", res.Errors)
	})
}
