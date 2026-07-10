package setup_test

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/harness/ibclink"
	"github.com/cosmos/ibc/link/harness/ibclink/wire"
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

func newRunner(t *testing.T, configPath string) ibclink.Runner {
	t.Helper()
	r, err := ibclink.NewRunner(ibclink.Options{ConfigPath: configPath})
	require.NoError(t, err)
	return r
}

func writeConfig(t *testing.T, c wire.ConfigYAML) string {
	t.Helper()
	data, err := c.Marshal()
	require.NoError(t, err)
	p := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(p, data, 0o600))
	return p
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
