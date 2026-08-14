// SPDX-License-Identifier: Apache-2.0

package main

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/config"
)

func TestConfigValidate(t *testing.T) {
	previous := globalFlags
	t.Cleanup(func() { globalFlags = previous })

	home := t.TempDir()
	t.Chdir(home)
	globalFlags = defaultFlagSet()
	globalFlags.Home = home
	globalFlags.Quiet = true

	require.NoError(t, writeConfigFile(filepath.Join(home, globalFlags.Config), config.Default(), nil))
	require.NoError(t, configValidate(nil, nil))
}
