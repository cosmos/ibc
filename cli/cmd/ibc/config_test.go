// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"path/filepath"
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
	previous := []string{version, commit, contractsVersion, ibcGoVersion}
	t.Cleanup(func() {
		version, commit, contractsVersion, ibcGoVersion = previous[0], previous[1], previous[2], previous[3]
	})

	// As set by `make build` for a release.
	version, commit = "cli/v0.2.0", "b1a4cf0a1546"
	contractsVersion, ibcGoVersion = "v0.0.0-20261028174625-daaed552e739", "v11.2.0"
	require.Equal(t, "ibc cli version cli/v0.2.0, commit: b1a4cf0a1546\n"+
		"ibc-contracts: v0.0.0-20261028174625-daaed552e739, commit: daaed552e739\n"+
		"ibc-go: v11.2.0, commit: unknown", versionInfo())

	// As built without `make build`.
	version, commit, contractsVersion, ibcGoVersion = "devel", "unknown", "unknown", "unknown"
	require.Equal(t, "ibc cli version devel, commit: unknown\n"+
		"ibc-contracts: unknown, commit: unknown\n"+
		"ibc-go: unknown, commit: unknown", versionInfo())
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
