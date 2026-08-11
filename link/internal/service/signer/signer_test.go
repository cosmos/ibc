package signer

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/keyfile"
)

func TestSignerSet(t *testing.T) {
	t.Run("setGet", func(t *testing.T) {
		// ARRANGE
		signerSet := NewSet()

		signerA, err := GenerateLocalEd25519Signer()
		require.NoError(t, err)

		signerSet.Set("A", signerA)

		// ACT
		actualSigner, found := signerSet.Get("A")
		missingSigner, missingFound := signerSet.Get("E")

		// ASSERT
		require.True(t, found)
		require.Same(t, signerA, actualSigner)
		require.Nil(t, missingSigner)
		require.False(t, missingFound)
	})
}

func TestNewSignerFromConfigExpandsHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	key, err := GenerateLocalEd25519Signer()
	require.NoError(t, err)

	path := filepath.Join(home, "keys", "signer.json")
	require.NoError(t, key.StoreToFile(path))

	loadedSigner, alias, err := NewSignerFromConfig(context.Background(), config.SignerConfig{
		Alias: "local",
		Type:  config.SignerLocal,
		File:  "~/keys/signer.json",
	})

	require.NoError(t, err)
	require.Equal(t, "local", alias)
	require.Equal(t, key.PublicKey(), loadedSigner.PublicKey())
}

func TestNewSignerFromConfigRequiresExactFilePath(t *testing.T) {
	key, err := GenerateLocalEd25519Signer()
	require.NoError(t, err)

	dir := t.TempDir()
	require.NoError(t, key.StoreToFile(filepath.Join(dir, "signer.json")))

	_, _, err = NewSignerFromConfig(context.Background(), config.SignerConfig{
		Alias: "local",
		Type:  config.SignerLocal,
		File:  filepath.Join(dir, "signer"),
	})
	require.Error(t, err)
}

func TestEVMAddressOf(t *testing.T) {
	key, err := GenerateLocalKey(keyfile.ECDSA)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "deployer.json")
	require.NoError(t, key.StoreToFile(path))
	want, err := PublicKeyToEVMAddress(key.PublicKey())
	require.NoError(t, err)

	got, err := EVMAddressOf(config.SignerConfig{Alias: "d", Type: config.SignerLocal, File: path})
	require.NoError(t, err)
	require.Equal(t, want, got)

	_, err = EVMAddressOf(config.SignerConfig{Alias: "kms", Type: config.SignerRemote})
	require.ErrorContains(t, err, "remote signer")
}
