package signer

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"path/filepath"
	"testing"

	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalSecp256k1Signer(t *testing.T) {
	ctx := context.Background()
	message := []byte("hello secp256k1")
	digest := sha256.Sum256(message)

	t.Run("happyPath", func(t *testing.T) {
		// ARRANGE
		signer, err := GenerateLocalSecp256k1Signer()
		require.NoError(t, err)

		// ACT
		signature, err := signer.Sign(ctx, digest[:])

		// ASSERT
		require.NoError(t, err)
		assertRecoverableSignature(t, signer.PublicKey(), digest[:], signature)

		// ARRANGE
		// Given exported private key
		privateKey := signer.PrivateKey()

		// ACT
		// Re-use the same PK and check PubKeys are the same
		importedSigner, err := NewLocalSecp256k1Signer(privateKey)

		// ASSERT
		require.NoError(t, err)
		assert.Equal(t, signer.PublicKey(), importedSigner.PublicKey())

		t.Run("fileExportImport", func(t *testing.T) {
			// ARRANGE
			path := filepath.Join(t.TempDir(), "secp256k1.json")

			// ACT
			err := signer.StoreToFile(path)

			// ASSERT
			require.NoError(t, err)

			// ACT
			importedKey, err := LocalKeyFromFile(path)

			// ASSERT
			require.NoError(t, err)
			assert.Equal(t, signer.PublicKey(), importedKey.PublicKey())
		})
	})

	t.Run("digestRequired", func(t *testing.T) {
		// ARRANGE
		signer, err := GenerateLocalSecp256k1Signer()
		require.NoError(t, err)

		// ACT
		_, err = signer.Sign(ctx, message)

		// ASSERT
		require.ErrorContains(t, err, "digest must be 32 bytes")
	})

	t.Run("fileImportInvalidKey", func(t *testing.T) {
		t.Run("eddsaKey", func(t *testing.T) {
			// ARRANGE
			ed25519Signer, err := GenerateLocalEd25519Signer()
			require.NoError(t, err)

			path := writeFileJSON(t, "secp256k1.json", keyFile{
				Type:       ECDSA,
				PrivateKey: base64.StdEncoding.EncodeToString(ed25519Signer.PrivateKey()),
			})

			// ACT
			_, err = LocalKeyFromFile(path)

			// ASSERT
			require.Error(t, err)
		})

		t.Run("notFound", func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "missing.json")
			_, err := LocalKeyFromFile(path)
			require.Error(t, err)
		})

		t.Run("bytesMismatch", func(t *testing.T) {
			path := writeFileJSON(t, "secp256k1.json", keyFile{
				Type:       ECDSA,
				PrivateKey: base64.StdEncoding.EncodeToString([]byte("too short")),
			})

			_, err := LocalKeyFromFile(path)
			require.Error(t, err)
		})
	})
}

func assertRecoverableSignature(t *testing.T, pubKey []byte, digest []byte, signature []byte) {
	t.Helper()

	require.Len(t, signature, 65)
	r, sig, v := signature[0:32], signature[32:64], signature[64]
	require.True(t, v == 0 || v == 1, "v must be a 0/1 recovery id")

	compact := make([]byte, 65)
	compact[0] = 27 + v
	copy(compact[1:33], r)
	copy(compact[33:65], sig)

	recovered, _, err := ecdsa.RecoverCompact(compact, digest)
	require.NoError(t, err)
	assert.Equal(t, pubKey, recovered.SerializeCompressed())
}
