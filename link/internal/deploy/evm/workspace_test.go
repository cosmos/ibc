package evm

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnsureWorkspaceExtracts(t *testing.T) {
	home := t.TempDir()

	ws, err := extractWorkspace(home) // extraction only; no bun/forge needed
	require.NoError(t, err)

	for _, f := range []string{
		"foundry.toml",
		"remappings.txt",
		"package.json",
		"scripts/DeployCore.s.sol",
		"scripts/DeployAttestationClient.s.sol",
		"release-bytecode/ICS26Router.json",
		"release-bytecode/AttestationLightClient.json",
	} {
		_, err := os.Stat(filepath.Join(ws.Dir, f))
		require.NoError(t, err, f)
	}

	// staged artifacts carry real creation bytecode
	bz, err := os.ReadFile(filepath.Join(ws.Dir, "release-bytecode/ICS26Router.json"))
	require.NoError(t, err)
	require.Contains(t, string(bz), `"object":"0x`)

	// re-extraction over an existing dir succeeds (idempotent)
	_, err = extractWorkspace(home)
	require.NoError(t, err)
}

func TestParseScriptReturns(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "broadcast", "DeployCore.s.sol", "31337")
	require.NoError(t, os.MkdirAll(target, 0o755))
	fixture, err := os.ReadFile("testdata/run-latest.json")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(target, "run-latest.json"), fixture, 0o644))

	returns, txs, err := parseScriptReturns(dir, "DeployCore.s.sol", "31337")
	require.NoError(t, err)
	require.Equal(t, "0x00000000000000000000000000000000000000cc", returns["ics26Router"])
	require.Len(t, txs, 2)
}

func TestParseScriptReturnsDoubleEscaped(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "broadcast", "DeployCore.s.sol", "31337")
	require.NoError(t, os.MkdirAll(target, 0o755))
	fixture, err := os.ReadFile("testdata/run-latest-double-escaped.json")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(target, "run-latest.json"), fixture, 0o644))

	returns, txs, err := parseScriptReturns(dir, "DeployCore.s.sol", "31337")
	require.NoError(t, err)
	require.Equal(t, "0x00000000000000000000000000000000000000aa", returns["accessManager"])
	require.Equal(t, "0x00000000000000000000000000000000000000bb", returns["ics26RouterImplementation"])
	require.Equal(t, "0x00000000000000000000000000000000000000cc", returns["ics26Router"])
	require.Len(t, txs, 2)
}

func TestParseScriptReturnsInvalidJSONWithoutDoubleEscapeSignature(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "broadcast", "DeployCore.s.sol", "31337")
	require.NoError(t, os.MkdirAll(target, 0o755))
	// "value" is malformed JSON but contains no `\"` double-escape signature,
	// so the fallback must not fire: this should error, not silently retry
	// into a bogus success.
	run := `{
  "transactions": [],
  "returns": {
    "0": { "internal_type": "string", "value": "{not valid json" }
  }
}`
	require.NoError(t, os.WriteFile(filepath.Join(target, "run-latest.json"), []byte(run), 0o644))

	_, _, err := parseScriptReturns(dir, "DeployCore.s.sol", "31337")
	require.ErrorContains(t, err, "parse script return JSON")
}

func TestBuildScriptArgs(t *testing.T) {
	args := buildScriptArgs(ScriptOptions{
		Script: "scripts/DeployCore.s.sol:DeployCore",
		RPCURL: "http://localhost:8545",
	})
	require.Equal(t, []string{
		"script", "scripts/DeployCore.s.sol:DeployCore",
		"--rpc-url", "http://localhost:8545",
		"--broadcast", "--slow", "--non-interactive",
	}, args)
	_ = context.Background()
}
