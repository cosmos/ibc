// SPDX-License-Identifier: Apache-2.0

package signer

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/cometbft/cometbft/crypto/ed25519"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalEd25519Signer(t *testing.T) {
	ctx := context.Background()
	message := []byte("hello ed25519")

	t.Run("happyPath", func(t *testing.T) {
		// ARRANGE
		signer, err := GenerateLocalEd25519Signer()
		require.NoError(t, err)

		// ACT
		signature, err := signer.Sign(ctx, message)

		// ASSERT
		require.NoError(t, err)
		assert.True(t, ed25519.PubKey(signer.PublicKey()).VerifySignature(message, signature))

		// ARRANGE
		// Given exported private key
		privateKey := signer.PrivateKey()

		// ACT
		// Re-use the same PK and check PubKeys are the same
		importedSigner, err := NewLocalEd25519Signer(privateKey)

		// ASSERT
		require.NoError(t, err)
		assert.Equal(t, signer.PublicKey(), importedSigner.PublicKey())

		t.Run("fileExportImport", func(t *testing.T) {
			// ARRANGE
			path := filepath.Join(t.TempDir(), "ed25519.json")

			// ACT
			err := signer.StoreToFile(path)

			// ASSERT
			require.NoError(t, err)
			info, err := os.Stat(path)
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

			// ACT
			importedKey, err := LocalKeyFromFile(path)

			// ASSERT
			require.NoError(t, err)
			assert.Equal(t, signer.PublicKey(), importedKey.PublicKey())
		})
	})

	t.Run("fileImportInvalidKey", func(t *testing.T) {
		ecdsaSigner, err := GenerateLocalSecp256k1Signer()
		require.NoError(t, err)

		testFileImportInvalidContents(t, string(EDDSA), ecdsaSigner.PrivateKey())
	})
}

func TestLocalKeyFromFileNotFound(t *testing.T) {
	_, err := LocalKeyFromFile(filepath.Join(t.TempDir(), "missing.json"))
	require.Error(t, err)
}

func testFileImportInvalidContents(t *testing.T, fileType string, wrongTypeKey []byte) {
	t.Helper()

	t.Run("wrongKeyType", func(t *testing.T) {
		path := writeFileJSON(t, "key.json", map[string]string{
			"type":             fileType,
			"privateKeyBase64": base64.StdEncoding.EncodeToString(wrongTypeKey),
		})
		_, err := LocalKeyFromFile(path)
		require.Error(t, err)
	})

	t.Run("bytesMismatch", func(t *testing.T) {
		path := writeFileJSON(t, "key.json", map[string]string{
			"type":             fileType,
			"privateKeyBase64": base64.StdEncoding.EncodeToString([]byte("too short")),
		})
		_, err := LocalKeyFromFile(path)
		require.Error(t, err)
	})
}

func writeFileJSON(t *testing.T, name string, data any) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)

	bz, err := json.Marshal(data)
	require.NoError(t, err)

	err = os.WriteFile(path, bz, 0o644)
	require.NoError(t, err)

	return path
}
