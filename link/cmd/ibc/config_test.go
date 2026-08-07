package main

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/config"
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
