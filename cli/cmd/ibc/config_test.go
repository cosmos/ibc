// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"path/filepath"
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/internal/config"
)

func TestConfigValidate(t *testing.T) {
	previous := globalFlags
	t.Cleanup(func() { globalFlags = previous })

	home := t.TempDir()
	t.Chdir(home)
	globalFlags = config.DefaultFlagSet()
	globalFlags.Home = home
	globalFlags.Quiet = true

	require.NoError(t, config.DefaultConfig().StoreToFile(filepath.Join(home, globalFlags.Config)))
	require.NoError(t, configValidate(nil, nil))
}

func TestVersionInfo(t *testing.T) {
	previousContracts, previousIBCGo := contractsVersion, ibcGoVersion
	t.Cleanup(func() { contractsVersion, ibcGoVersion = previousContracts, previousIBCGo })
	contractsVersion = "v0.0.0-20261028174625-daaed552e739"
	ibcGoVersion = "v11.2.0"

	info := &debug.BuildInfo{Settings: []debug.BuildSetting{
		{Key: "vcs.time", Value: "2026-10-30T09:56:11Z"},
		{Key: "vcs.revision", Value: "b1a4cf0a1546c5a45da40f5da1fad9a81d3a7eef"},
	}}
	require.Equal(t, "ibc cli version devel, commit: b1a4cf0a1546c5a45da40f5da1fad9a81d3a7eef\n"+
		"ibc-contracts: v0.0.0-20261028174625-daaed552e739, commit: daaed552e739\n"+
		"ibc-go: v11.2.0, commit: unknown", versionInfo(info, true))

	// Without build info, as when the binary is built outside a git checkout.
	contractsVersion, ibcGoVersion = "unknown", "unknown"
	require.Equal(t, "ibc cli version devel, commit: unknown\n"+
		"ibc-contracts: unknown, commit: unknown\n"+
		"ibc-go: unknown, commit: unknown", versionInfo(nil, false))
}

func TestVersionFlag(t *testing.T) {
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetArgs([]string{"--version"})
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetArgs(nil)
	})

	require.NoError(t, rootCmd.Execute())
	require.Equal(t, rootCmd.Version+"\n", out.String())
}
