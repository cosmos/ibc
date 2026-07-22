package e2e_test

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/ibclink"
	"github.com/cosmos/ibc/link/cmd/configcmd"
	"github.com/cosmos/ibc/link/keyfile"
)

func TestConfigValidation(t *testing.T) {
	env := e2etest.Start(t, e2etest.SelectedSuite(t))
	a, err := env.Chain(e2etest.ChainA)
	require.NoError(t, err)
	b, err := env.Chain(e2etest.ChainB)
	require.NoError(t, err)
	instanceA, err := env.IBCInstanceForChain(e2etest.ChainA)
	require.NoError(t, err)
	instanceB, err := env.IBCInstanceForChain(e2etest.ChainB)
	require.NoError(t, err)
	connection, err := env.Connection(env.Connections()[0])
	require.NoError(t, err)
	relayerSigner := writeLocalSigner(t, "relayer")
	driver, configPath := newEnvironmentDriver(t, env)

	valid := configcmd.Config{
		Signers: []configcmd.Signer{relayerSigner},
		Chains: []configcmd.Chain{
			{
				ID: string(e2etest.ChainA), Type: configcmd.ChainTypeEVM, ChainID: a.EVMChainID(),
				EVMSigner: relayerSigner.Alias, ICS26Router: string(instanceA.Locator()),
				RPC: chainRPC(t, driver, e2etest.ChainA),
			},
			{
				ID: string(e2etest.ChainB), Type: configcmd.ChainTypeEVM, ChainID: b.EVMChainID(),
				EVMSigner: relayerSigner.Alias, ICS26Router: string(instanceB.Locator()),
				RPC: chainRPC(t, driver, e2etest.ChainB),
			},
		},
		DB: configcmd.DB{Type: configcmd.DBTypeSQLite, URL: filepath.Join(t.TempDir(), "valid.db")},
		Relayer: configcmd.Relayer{Routes: []configcmd.Route{{
			ID: string(e2etest.RouteAtoB), Source: string(e2etest.ChainA), Destination: string(e2etest.ChainB),
			Type:         configcmd.RouteEVMToEVMAttested,
			SourceClient: string(connection.A().Locator()),
			DestClient:   string(connection.B().Locator()),
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
					ID:          string(e2etest.ChainA),
					Type:        configcmd.ChainTypeEVM,
					ChainID:     a.EVMChainID(),
					ICS26Router: string(instanceA.Locator()),
					RPC:         chainRPC(t, driver, e2etest.ChainA),
				},
			},
			DB: configcmd.DB{Type: configcmd.DBTypeSQLite, URL: filepath.Join(t.TempDir(), "bad.db")},
			Relayer: configcmd.Relayer{
				Routes: []configcmd.Route{
					{
						ID:           "route-bad",
						Source:       string(e2etest.ChainA),
						Destination:  "999999",
						Type:         configcmd.RouteEVMToEVMAttested,
						SourceClient: "client-a",
						DestClient:   "client-b",
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

	t.Run("missing_ics26_router_exit1", func(t *testing.T) {
		ctx := t.Context()
		driver, configPath := newEnvironmentDriver(t, env)
		bad := valid
		bad.Chains = append([]configcmd.Chain(nil), valid.Chains...)
		bad.Chains[0].ICS26Router = ""
		bad.DB.URL = filepath.Join(t.TempDir(), "missing-router.db")
		writeConfig(t, configPath, bad)

		res, err := driver.ValidateConfig(ctx, false)
		requireExit(t, err, ibclink.ErrConfigInvalid)
		require.NotNil(t, res)
		require.False(t, res.Valid)
		require.True(t, hasErrorPath(res.Errors, "chains[0].ics26Router"),
			"expected located ics26Router error, got %+v", res.Errors)
	})

	t.Run("missing_route_clients_exit1", func(t *testing.T) {
		ctx := t.Context()
		driver, configPath := newEnvironmentDriver(t, env)
		bad := valid
		bad.Relayer.Routes = append([]configcmd.Route(nil), valid.Relayer.Routes...)
		bad.Relayer.Routes[0].SourceClient = ""
		bad.Relayer.Routes[0].DestClient = ""
		bad.DB.URL = filepath.Join(t.TempDir(), "missing-clients.db")
		writeConfig(t, configPath, bad)

		res, err := driver.ValidateConfig(ctx, false)
		requireExit(t, err, ibclink.ErrConfigInvalid)
		require.NotNil(t, res)
		require.False(t, res.Valid)
		require.True(t, hasErrorPath(res.Errors, "relayer.routes[0].sourceClient"),
			"expected located sourceClient error, got %+v", res.Errors)
		require.True(t, hasErrorPath(res.Errors, "relayer.routes[0].destClient"),
			"expected located destClient error, got %+v", res.Errors)
	})

	t.Run("invalid_ics26_router_exit1", func(t *testing.T) {
		ctx := t.Context()
		driver, configPath := newEnvironmentDriver(t, env)
		bad := valid
		bad.Chains = append([]configcmd.Chain(nil), valid.Chains...)
		bad.Chains[0].ICS26Router = "not-an-address"
		bad.DB.URL = filepath.Join(t.TempDir(), "bad-router.db")
		writeConfig(t, configPath, bad)

		res, err := driver.ValidateConfig(ctx, false)
		requireExit(t, err, ibclink.ErrConfigInvalid)
		require.NotNil(t, res)
		require.False(t, res.Valid)
		require.True(t, hasErrorPath(res.Errors, "chains[0].ics26Router"),
			"expected located ics26Router error, got %+v", res.Errors)
	})

	t.Run("live_down_rpc_exit1", func(t *testing.T) {
		ctx := t.Context()
		driver, configPath := newDriver(t)
		down := configcmd.Config{
			Chains: []configcmd.Chain{
				{
					ID:          string(e2etest.ChainA),
					Type:        configcmd.ChainTypeEVM,
					ChainID:     a.EVMChainID(),
					ICS26Router: string(instanceA.Locator()),
					RPC:         configcmd.RPC{URL: refusingRPCURL(t)},
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
					ID:          string(e2etest.ChainA),
					Type:        configcmd.ChainTypeEVM,
					ChainID:     a.EVMChainID() + 99999,
					ICS26Router: string(instanceA.Locator()),
					RPC:         chainRPC(t, driver, e2etest.ChainA),
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

func requireExit(t *testing.T, err error, wantClass error) {
	t.Helper()
	require.Error(t, err, "command must fail")
	require.ErrorIs(t, err, wantClass, "failure class")
	var exitErr *ibclink.ExitError
	require.ErrorAs(t, err, &exitErr)
	require.Equal(t, 1, exitErr.Code, "all ibc command failures use exit code 1")
}

func hasErrorPath(errs []configcmd.ValidationError, path string) bool {
	for _, e := range errs {
		if e.Path == path {
			return true
		}
	}
	return false
}

func newDriver(t *testing.T) (*ibclink.Driver, string) {
	t.Helper()
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	driver, err := ibclink.NewDriver(configPath)
	require.NoError(t, err)
	return driver, configPath
}

func newEnvironmentDriver(t *testing.T, env *environment.Environment) (*ibclink.Driver, string) {
	t.Helper()
	driver, configPath := newDriver(t)
	require.NoError(t, env.BindIBCLink(driver))
	return driver, configPath
}

func chainRPC(t *testing.T, driver *ibclink.Driver, id environment.ChainID) configcmd.RPC {
	t.Helper()
	rpc, err := driver.ChainRPC(string(id))
	require.NoError(t, err)
	return rpc
}

func writeConfig(t *testing.T, path string, c configcmd.Config) {
	t.Helper()
	data, err := c.Marshal()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o600))
}

func writeLocalSigner(t *testing.T, alias string) configcmd.Signer {
	t.Helper()
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), alias+".json")
	require.NoError(t, keyfile.Store(path, keyfile.ECDSA, crypto.FromECDSA(key)))
	return configcmd.Signer{Alias: alias, Type: configcmd.SignerTypeLocal, File: path}
}

func refusingRPCURL(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	return "http://" + l.Addr().String()
}
