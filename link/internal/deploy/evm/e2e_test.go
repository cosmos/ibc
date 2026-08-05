package evm

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/deploy"
	"github.com/cosmos/ibc/link/internal/deploy/manifest"
)

// anvil's default funded key 0
const anvilKey = "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"

func startAnvil(t *testing.T, port int) string {
	t.Helper()
	cmd := exec.Command("anvil", "--port", fmt.Sprint(port))
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	rpc := fmt.Sprintf("http://127.0.0.1:%d", port)
	require.Eventually(t, func() bool {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 100*time.Millisecond)
		if err != nil {
			return false
		}
		conn.Close()
		return true
	}, 10*time.Second, 100*time.Millisecond)
	return rpc
}

func TestDeployE2E(t *testing.T) {
	for _, tool := range []string{"forge", "bun", "anvil"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not on PATH", tool)
		}
	}
	if os.Getenv("IBC_DEPLOY_E2E") != "1" {
		t.Skip("set IBC_DEPLOY_E2E=1 to run (network bun install, fixed anvil port); use `make test-deploy`")
	}
	if testing.Short() {
		t.Skip("short mode")
	}

	ctx := context.Background()
	home := t.TempDir()
	dir := home + "/deployments"
	rpc := startAnvil(t, 18545)

	driver, err := New(ctx, Options{ChainID: "31337", RPCURL: rpc, Home: home, DeployerKeyHex: anvilKey})
	require.NoError(t, err)

	deployer := driver.DeployerAddress()
	results, err := deploy.RunSteps(ctx, slog.Default(), false, deploy.CoreSteps(driver, dir, "31337", deployer))
	require.NoError(t, err)
	require.Equal(t, deploy.ActionExecuted, results[0].Action)

	spec := deploy.ClientSpec{
		ClientID:             "link-99",
		Type:                 deploy.ClientTypeAttestation,
		CounterpartyChainID:  "99",
		CounterpartyClientID: "link-31337",
		Params: deploy.AttestationParams{
			Attestors:        []string{deployer},
			Threshold:        1,
			InitialHeight:    1,
			InitialTimestamp: uint64(time.Now().Unix()),
		},
	}
	_, err = deploy.RunSteps(ctx, slog.Default(), false, deploy.ClientSteps(driver, dir, "31337", spec))
	require.NoError(t, err)

	// idempotency: everything skips on rerun
	results, err = deploy.RunSteps(ctx, slog.Default(), false,
		append(deploy.CoreSteps(driver, dir, "31337", deployer), deploy.ClientSteps(driver, dir, "31337", spec)...))
	require.NoError(t, err)
	for _, r := range results {
		require.Equal(t, deploy.ActionSkipped, r.Action, r.Name)
	}

	// import round-trip matches the deployed state
	m, err := manifest.Load(dir, "31337")
	require.NoError(t, err)
	discovered, err := driver.Discover(ctx, m.Core.Router)
	require.NoError(t, err)
	c, ok := discovered.Client("link-99")
	require.True(t, ok)
	recorded, _ := m.Client("link-99")
	require.Equal(t, recorded.Address, c.Address)

	// verify reports clean
	report, err := driver.Verify(ctx, m)
	require.NoError(t, err)
	require.Empty(t, report.Failed())
}
