package main

import (
	"path/filepath"
	"testing"

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

	if err := config.DefaultConfig().StoreToFile(filepath.Join(home, globalFlags.Config)); err != nil {
		t.Fatal(err)
	}
	if err := configValidate(nil, nil); err != nil {
		t.Fatal(err)
	}
}
