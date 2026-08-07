package keyfile_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/keyfile"
)

func TestSignerFileContract(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "keys")
	path := filepath.Join(dir, "signer.json")
	require.NoError(t, keyfile.Store(path, keyfile.ECDSA, []byte{1, 2, 3}))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	// the on-disk shape is a contract with external consumers; assert exact bytes
	//nolint:testifylint // JSONEq would not catch a switch to indented output
	require.Equal(t, `{"type":"ecdsa","privateKeyBase64":"AQID"}`, string(data))

	dirInfo, err := os.Stat(dir)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o700), dirInfo.Mode().Perm())

	fileInfo, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), fileInfo.Mode().Perm())

	keyType, privateKey, err := keyfile.Load(path)
	require.NoError(t, err)
	require.Equal(t, keyfile.ECDSA, keyType)
	require.Equal(t, []byte{1, 2, 3}, privateKey)

	require.Error(t, keyfile.Store(path, keyfile.ECDSA, []byte{4, 5, 6}))
}

func TestParseTypeRejectsUnknownType(t *testing.T) {
	t.Parallel()

	_, err := keyfile.ParseType("rsa")
	require.EqualError(t, err, "invalid key type: rsa")
}

func TestLoadRejectsMalformedCredential(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"invalid JSON":   `{`,
		"invalid type":   `{"type":"rsa","privateKeyBase64":"AQID"}`,
		"invalid base64": `{"type":"ecdsa","privateKeyBase64":"%%%"}`,
	}
	for name, credential := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "signer.json")
			require.NoError(t, os.WriteFile(path, []byte(credential), 0o600))
			_, _, err := keyfile.Load(path)
			require.Error(t, err)
		})
	}
}
