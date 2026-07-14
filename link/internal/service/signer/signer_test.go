package signer

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/cosmos/kms/gen/signerservice"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/config"
)

func TestSignerSet(t *testing.T) {
	t.Run("setGet", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		signerSet := NewSet()

		signerA, err := GenerateLocalEd25519Signer()
		require.NoError(t, err)

		signerB, err := GenerateLocalSecp256k1Signer()
		require.NoError(t, err)

		signerC, err := GenerateLocalEd25519Signer()
		require.NoError(t, err)

		remoteTS := newRemoteTestSuite(t)
		remoteTS.OnKeyRequest("remote-key", &signerservice.GetKeyResponse{
			Key: &signerservice.Key{
				Id:     "remote-key",
				Pubkey: []byte("remote-public-key"),
				Scheme: signerservice.SignatureScheme_ED25519,
			},
		}, nil)

		remoteSignerD, err := NewRemote(ctx, remoteTS.Client, "remote-key")
		require.NoError(t, err)

		signerSet.Set("A", signerA)
		signerSet.Set("B", signerB)
		signerSet.Set("C", signerC)
		signerSet.Set("D", remoteSignerD)

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
