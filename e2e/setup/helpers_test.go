package setup_test

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/api/v2/keyfile"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/ibclink"
	"github.com/cosmos/ibc/e2e/internal/harness/ibclink/wire"
)

func requireExit(t *testing.T, err error, wantClass error, wantCode int) {
	t.Helper()
	require.Error(t, err, "command must fail")
	require.ErrorIs(t, err, wantClass, "failure class")
	var exitErr *ibclink.ExitError
	require.ErrorAs(t, err, &exitErr)
	require.Equal(t, wantCode, exitErr.Code, "exit code")
}

func hasErrorPath(errs []wire.ValidationError, path string) bool {
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

func chainRPC(t *testing.T, driver *ibclink.Driver, id environment.ChainID) wire.RPC {
	t.Helper()
	rpc, err := driver.ChainRPC(string(id))
	require.NoError(t, err)
	return rpc
}

func writeConfig(t *testing.T, path string, c wire.ConfigYAML) {
	t.Helper()
	data, err := c.Marshal()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o600))
}

func writeLocalSigner(t *testing.T, alias string) wire.Signer {
	t.Helper()
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), alias+".json")
	require.NoError(t, keyfile.Store(path, keyfile.ECDSA, crypto.FromECDSA(key)))
	return wire.Signer{Alias: alias, Type: wire.SignerTypeLocal, File: path}
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
