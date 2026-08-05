// Package evm implements the deploy Target for EVM chains: contract creation
// via an embedded Foundry workspace, IBC wiring via go-abigen bindings.
package evm

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/attestation"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
)

//go:embed forgeproject
var projectFS embed.FS

// contractsVersion identifies the pinned contract artifacts. Keep in sync
// with the go-abigen version in go.mod.
const contractsVersion = "go-abigen v0.0.0-20260618122836-39904319467b"

// Workspace is the on-disk Foundry project the driver runs scripts from.
type Workspace struct {
	Dir string
}

// EnsureTools verifies the external tools the EVM driver shells out to.
func EnsureTools() error {
	for _, tool := range []string{"forge", "bun"} {
		if _, err := exec.LookPath(tool); err != nil {
			return fmt.Errorf("%s not found on PATH: the EVM deployment driver requires foundry and bun", tool)
		}
	}
	return nil
}

// EnsureWorkspace extracts the embedded forge project under home, stages the
// pinned contract bytecode, and installs JS dependencies when missing.
func EnsureWorkspace(ctx context.Context, home string) (*Workspace, error) {
	ws, err := extractWorkspace(home)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(ws.Dir, "node_modules")); os.IsNotExist(err) {
		install := exec.CommandContext(ctx, "bun", "install", "--frozen-lockfile")
		install.Dir = ws.Dir
		if out, err := install.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("bun install in %s: %w\n%s", ws.Dir, err, out)
		}
	}
	return ws, nil
}

// extractWorkspace writes the embedded project and bytecode artifacts to
// <home>/deploy/forge. Always overwrites project files (upgrade-safe) and
// leaves node_modules/broadcast alone.
func extractWorkspace(home string) (*Workspace, error) {
	dir := filepath.Join(home, "deploy", "forge")
	err := fs.WalkDir(projectFS, "forgeproject", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel := strings.TrimPrefix(path, "forgeproject/")
		target := filepath.Join(dir, rel)
		if mkErr := os.MkdirAll(filepath.Dir(target), 0o755); mkErr != nil {
			return mkErr
		}
		bz, readErr := projectFS.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		return os.WriteFile(target, bz, 0o644)
	})
	if err != nil {
		return nil, fmt.Errorf("extract forge workspace: %w", err)
	}
	if err := stageBytecode(dir); err != nil {
		return nil, err
	}
	return &Workspace{Dir: dir}, nil
}

// stageBytecode writes creation bytecode from the pinned go-abigen artifacts
// in the foundry artifact shape vm.getCode reads.
func stageBytecode(dir string) error {
	artifacts := map[string]string{
		"ICS26Router.json":            ics26router.ContractMetaData.Bin,
		"AttestationLightClient.json": attestation.ContractMetaData.Bin,
	}
	out := filepath.Join(dir, "release-bytecode")
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	for name, bin := range artifacts {
		if bin == "" {
			return fmt.Errorf("go-abigen artifact %s has no creation bytecode (%s)", name, contractsVersion)
		}
		artifact := map[string]any{"bytecode": map[string]string{"object": bin}}
		bz, err := json.Marshal(artifact)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(out, name), bz, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// ScriptOptions parameterizes one forge script invocation.
type ScriptOptions struct {
	// Script is the forge target, e.g. "scripts/DeployCore.s.sol:DeployCore".
	Script string
	// ScriptFile is the file component, e.g. "DeployCore.s.sol" (broadcast dir key).
	ScriptFile string
	RPCURL     string
	ChainID    string
	// PrivateKeyHex is passed via the IBC_DEPLOYER_KEY env var, never argv.
	PrivateKeyHex string
	Env           map[string]string
}

func buildScriptArgs(opts ScriptOptions) []string {
	return []string{
		"script", opts.Script,
		"--rpc-url", opts.RPCURL,
		"--broadcast", "--slow", "--non-interactive",
	}
}

// RunScript runs a forge script and returns its JSON return map plus the
// broadcast transaction hashes.
func (w *Workspace) RunScript(ctx context.Context, opts ScriptOptions) (map[string]string, []string, error) {
	cmd := exec.CommandContext(ctx, "forge", buildScriptArgs(opts)...)
	cmd.Dir = w.Dir
	cmd.Env = append(os.Environ(), "IBC_DEPLOYER_KEY=0x"+strings.TrimPrefix(opts.PrivateKeyHex, "0x"))
	for k, v := range opts.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return nil, nil, fmt.Errorf("forge script %s: %w\n%s", opts.Script, err, out.String())
	}
	return parseScriptReturns(w.Dir, opts.ScriptFile, opts.ChainID)
}

// parseScriptReturns reads broadcast/<script>/<chainid>/run-latest.json and
// decodes the script's JSON string return value.
func parseScriptReturns(projectDir, scriptFile, chainID string) (map[string]string, []string, error) {
	path := filepath.Join(projectDir, "broadcast", scriptFile, chainID, "run-latest.json")
	bz, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read forge broadcast output: %w", err)
	}
	var run struct {
		Transactions []struct {
			Hash string `json:"hash"`
		} `json:"transactions"`
		Returns map[string]struct {
			Value string `json:"value"`
		} `json:"returns"`
	}
	if err := json.Unmarshal(bz, &run); err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	ret, ok := run.Returns["0"]
	if !ok {
		return nil, nil, fmt.Errorf("%s has no script return value", path)
	}
	returns := map[string]string{}
	value := ret.Value
	if err := json.Unmarshal([]byte(value), &returns); err != nil {
		// forge (as of 1.2.3) double-escapes string returns that themselves
		// contain quotes (e.g. our scripts' stdJson-serialized return
		// value): the recorded "value" has each `"` rendered as the
		// literal two bytes `\"` on top of normal JSON encoding, instead of
		// being embedded as valid JSON text. Only retry when that signature
		// is actually present, so an unrelated malformed value fails fast
		// with the original error instead of masking it with a blind retry.
		if !strings.Contains(value, `\"`) {
			return nil, nil, fmt.Errorf("parse script return JSON: %w", err)
		}
		unescaped := strings.ReplaceAll(value, `\"`, `"`)
		if err2 := json.Unmarshal([]byte(unescaped), &returns); err2 != nil {
			return nil, nil, fmt.Errorf("parse script return JSON: %w", err)
		}
	}
	txs := make([]string, 0, len(run.Transactions))
	for _, tx := range run.Transactions {
		txs = append(txs, tx.Hash)
	}
	return returns, txs, nil
}
