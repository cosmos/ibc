package setup_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/ibclink"
	"github.com/cosmos/ibc/e2e/internal/synthetic"
	"github.com/cosmos/ibc/link/cmd/configcmd"
)

func TestValidateConfig_SelectedSuite(t *testing.T) {
	env := e2etest.Start(t, e2etest.SelectedSuite(t))
	a, err := env.Chain(e2etest.ChainA)
	require.NoError(t, err)
	b, err := env.Chain(e2etest.ChainB)
	require.NoError(t, err)
	relayerSigner := writeLocalSigner(t, "relayer")
	driver, configPath := newEnvironmentDriver(t, env)

	valid := configcmd.Config{
		Signers: []configcmd.Signer{relayerSigner},
		Chains: []configcmd.Chain{
			{
				ID: string(e2etest.ChainA), Type: configcmd.ChainTypeEVM, ChainID: a.EVMChainID(),
				EVMSigner: relayerSigner.Alias, RPC: chainRPC(t, driver, e2etest.ChainA),
			},
			{
				ID: string(e2etest.ChainB), Type: configcmd.ChainTypeEVM, ChainID: b.EVMChainID(),
				EVMSigner: relayerSigner.Alias, RPC: chainRPC(t, driver, e2etest.ChainB),
			},
		},
		DB: configcmd.DB{Type: configcmd.DBTypeSQLite, URL: filepath.Join(t.TempDir(), "valid.db")},
		Relayer: configcmd.Relayer{Routes: []configcmd.Route{{
			ID: string(synthetic.RouteAtoB), Source: string(e2etest.ChainA), Destination: string(e2etest.ChainB),
			Type: configcmd.RouteEVMToEVMAttested,
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

	t.Run("invalid_config_exit1", func(t *testing.T) {
		ctx := t.Context()
		driver, configPath := newEnvironmentDriver(t, env)
		bad := configcmd.Config{
			Chains: []configcmd.Chain{
				{
					ID:      string(e2etest.ChainA),
					Type:    configcmd.ChainTypeEVM,
					ChainID: a.EVMChainID(),
					RPC:     chainRPC(t, driver, e2etest.ChainA),
				},
			},
			DB: configcmd.DB{Type: configcmd.DBTypeSQLite, URL: filepath.Join(t.TempDir(), "bad.db")},
			Relayer: configcmd.Relayer{
				Routes: []configcmd.Route{
					{
						ID:          "route-bad",
						Source:      string(e2etest.ChainA),
						Destination: "999999",
						Type:        configcmd.RouteEVMToEVMAttested,
					},
				},
			},
		}
		writeConfig(t, configPath, bad)

		res, err := driver.ValidateConfig(ctx, false)
		requireExit(t, err, ibclink.ErrConfigInvalid)
		require.NotNil(t, res)
		require.False(t, res.Valid)
		require.True(t, hasErrorPath(res.Errors, "relayer.routes[0].destination"),
			"expected located validation error, got %+v", res.Errors)
	})

	t.Run("live_down_rpc_exit1", func(t *testing.T) {
		ctx := t.Context()
		driver, configPath := newDriver(t)
		down := configcmd.Config{
			Chains: []configcmd.Chain{
				{
					ID:      string(e2etest.ChainA),
					Type:    configcmd.ChainTypeEVM,
					ChainID: a.EVMChainID(),
					RPC:     configcmd.RPC{URL: refusingRPCURL(t)},
				},
			},
			DB: configcmd.DB{Type: configcmd.DBTypeSQLite, URL: filepath.Join(t.TempDir(), "down.db")},
		}
		writeConfig(t, configPath, down)

		res, err := driver.ValidateConfig(ctx, true)
		requireExit(t, err, ibclink.ErrRPCUnreachable)
		require.NotNil(t, res)
		require.True(t, res.Valid)
		require.True(t, hasErrorPath(res.Errors, "chains[0].rpc.url"),
			"expected located RPC error, got %+v", res.Errors)
	})

	t.Run("live_wrong_chain_id_exit1", func(t *testing.T) {
		ctx := t.Context()
		driver, configPath := newEnvironmentDriver(t, env)
		wrongID := configcmd.Config{
			Chains: []configcmd.Chain{
				{
					ID:      string(e2etest.ChainA),
					Type:    configcmd.ChainTypeEVM,
					ChainID: a.EVMChainID() + 99999,
					RPC:     chainRPC(t, driver, e2etest.ChainA),
				},
			},
			DB: configcmd.DB{Type: configcmd.DBTypeSQLite, URL: filepath.Join(t.TempDir(), "wrongid.db")},
		}
		writeConfig(t, configPath, wrongID)

		structural, err := driver.ValidateConfig(ctx, false)
		require.NoError(t, err)
		require.True(t, structural.Valid)

		res, err := driver.ValidateConfig(ctx, true)
		requireExit(t, err, ibclink.ErrRPCUnreachable)
		require.NotNil(t, res)
		require.True(t, res.Valid)
		require.True(t, hasErrorPath(res.Errors, "chains[0].chainId"),
			"expected located chain-id error, got %+v", res.Errors)
	})
}
