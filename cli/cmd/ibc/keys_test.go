// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/internal/config"
	"github.com/cosmos/ibc/cli/internal/service/signer"
)

func TestSaveNamedKeyPopulatesConfig(t *testing.T) {
	useKeyTestHome(t)

	key, err := signer.GenerateLocalSecp256k1Signer()
	require.NoError(t, err)
	keyPath, err := saveNamedKey(key, "alice", true)
	require.NoError(t, err)
	require.FileExists(t, keyPath)

	configPath, err := globalFlags.ConfigPath()
	require.NoError(t, err)
	cfg, err := config.LoadFromFile(configPath, false, false)
	require.NoError(t, err)
	require.Equal(t, config.Signers{{
		Alias: "alice",
		Type:  config.SignerLocal,
		File:  "alice",
	}}, cfg.Signers)
}

func TestSaveNamedKeyRejectsExistingSigner(t *testing.T) {
	useKeyTestHome(t)

	cfg := config.DefaultConfig()
	cfg.Signers = config.Signers{{
		Alias:       "alice",
		Type:        config.SignerRemote,
		GRPC:        "localhost:3000",
		RemoteKeyID: "alice",
	}}
	configPath, err := globalFlags.ConfigPath()
	require.NoError(t, err)
	require.NoError(t, cfg.StoreToFile(configPath))

	key, err := signer.GenerateLocalSecp256k1Signer()
	require.NoError(t, err)
	_, err = saveNamedKey(key, "alice", true)
	require.ErrorContains(t, err, "signer alias already exists")

	keyPath, err := signer.KeyFilePath(globalFlags.Home, "alice")
	require.NoError(t, err)
	require.NoFileExists(t, keyPath)
}

func TestSaveNamedKeyRollsBackAfterConfigWriteFailure(t *testing.T) {
	home := useKeyTestHome(t)

	configPath, err := globalFlags.ConfigPath()
	require.NoError(t, err)
	require.NoError(t, config.DefaultConfig().StoreToFile(configPath))
	require.NoError(t, os.MkdirAll(filepath.Join(home, "keys"), 0o700))
	require.NoError(t, os.Chmod(home, 0o500))
	t.Cleanup(func() { require.NoError(t, os.Chmod(home, 0o700)) })

	if probe, probeErr := os.CreateTemp(home, "probe"); probeErr == nil {
		_ = probe.Close()
		_ = os.Remove(probe.Name())
		t.Skip("filesystem permits writes to a read-only directory")
	}

	key, err := signer.GenerateLocalSecp256k1Signer()
	require.NoError(t, err)
	_, err = saveNamedKey(key, "alice", true)
	require.Error(t, err)

	keyPath, err := signer.KeyFilePath(home, "alice")
	require.NoError(t, err)
	require.NoFileExists(t, keyPath)
}

func TestSaveNamedKeyRejectsInvalidConfig(t *testing.T) {
	useKeyTestHome(t)

	configPath, err := globalFlags.ConfigPath()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(configPath, []byte("\tinvalid"), 0o600))

	key, err := signer.GenerateLocalSecp256k1Signer()
	require.NoError(t, err)
	_, err = saveNamedKey(key, "alice", true)
	require.Error(t, err)

	keyPath, err := signer.KeyFilePath(globalFlags.Home, "alice")
	require.NoError(t, err)
	require.NoFileExists(t, keyPath)
}

func useKeyTestHome(t *testing.T) string {
	t.Helper()

	previousFlags := globalFlags
	t.Cleanup(func() { globalFlags = previousFlags })

	home := t.TempDir()
	globalFlags = config.DefaultFlagSet()
	globalFlags.Home = home
	return home
}
