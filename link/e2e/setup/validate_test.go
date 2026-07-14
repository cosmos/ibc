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
	driver, configPath := newEnvironmentDriver(t, env)

	valid := wire.ConfigYAML{
		Signers: []wire.Signer{relayerSigner},
		Chains: []wire.Chain{
			{
				ID: string(e2etest.ChainA), Type: wire.ChainTypeEVM, ChainID: a.EVMChainID(),
				EVMSigner: relayerSigner.Alias, RPC: chainRPC(t, driver, e2etest.ChainA),
			},
			{
				ID: string(e2etest.ChainB), Type: wire.ChainTypeEVM, ChainID: b.EVMChainID(),
				EVMSigner: relayerSigner.Alias, RPC: chainRPC(t, driver, e2etest.ChainB),
			},
		},
		DB: wire.DB{Type: wire.DBTypeSQLite, URL: filepath.Join(t.TempDir(), "valid.db")},
		Relayer: wire.Relayer{Routes: []wire.Route{{
			ID: string(synthetic.RouteAtoB), Source: string(e2etest.ChainA), Destination: string(e2etest.ChainB),
			Type: wire.RouteEVMToEVMAttested,
		}}},
	}
	writeConfig(t, configPath, valid)

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
		driver, configPath := newEnvironmentDriver(t, env)
		bad := wire.ConfigYAML{
			Chains: []wire.Chain{
				{
					ID:      string(e2etest.ChainA),
					Type:    wire.ChainTypeEVM,
					ChainID: a.EVMChainID(),
					RPC:     chainRPC(t, driver, e2etest.ChainA),
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
		writeConfig(t, configPath, bad)

		res, err := driver.ValidateConfig(ctx, false)
		requireExit(t, err, ibclink.ErrConfigInvalid, wire.ExitConfigInvalid)
		require.NotNil(t, res)
		require.False(t, res.Valid)
		require.True(t, hasErrorPath(res.Errors, "relayer.routes[0].destination"),
			"expected located validation error, got %+v", res.Errors)
	})

	t.Run("live_down_rpc_exit65", func(t *testing.T) {
		ctx := t.Context()
		driver, configPath := newDriver(t)
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
		writeConfig(t, configPath, down)

		res, err := driver.ValidateConfig(ctx, true)
		requireExit(t, err, ibclink.ErrRPCUnreachable, wire.ExitRPCUnreachable)
		require.NotNil(t, res)
		require.True(t, res.Valid)
		require.True(t, hasErrorPath(res.Errors, "chains[0].rpc.url"),
			"expected located RPC error, got %+v", res.Errors)
	})

	t.Run("live_wrong_chain_id_exit65", func(t *testing.T) {
		ctx := t.Context()
		driver, configPath := newEnvironmentDriver(t, env)
		wrongID := wire.ConfigYAML{
			Chains: []wire.Chain{
				{
					ID:      string(e2etest.ChainA),
					Type:    wire.ChainTypeEVM,
					ChainID: a.EVMChainID() + 99999,
					RPC:     chainRPC(t, driver, e2etest.ChainA),
				},
			},
			DB: wire.DB{Type: wire.DBTypeSQLite, URL: filepath.Join(t.TempDir(), "wrongid.db")},
		}
		writeConfig(t, configPath, wrongID)

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
