package main

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/config"
)

func TestLoadEffectiveConfigAppliesDBOverride(t *testing.T) {
	originalFlags := globalFlags
	t.Cleanup(func() {
		globalFlags = originalFlags
	})

	home := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.DB.Type = "invalid"
	cfg.DB.URL = ""

	require.NoError(t, cfg.StoreToFile(filepath.Join(home, "ibc.yml")))

	globalFlags = config.DefaultFlagSet()
	globalFlags.Home = home
	globalFlags.Config = "ibc.yml"
	globalFlags.DB = "postgres://user:pass@localhost:5432/override"

	got, err := loadEffectiveConfig(true)
	require.NoError(t, err)

	assert.Equal(t, config.DBTypePostgres, got.DB.Type)
	assert.Equal(t, globalFlags.DB, got.DB.URL)
}
