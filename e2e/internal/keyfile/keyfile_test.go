package keyfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLocalSignerFileContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "signer.json")
	require.NoError(t, Store(path, ECDSA, []byte{1, 2, 3}))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"ecdsa","privateKeyBase64":"AQID"}`, string(data))

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	keyType, privateKey, err := Load(path)
	require.NoError(t, err)
	require.Equal(t, ECDSA, keyType)
	require.Equal(t, []byte{1, 2, 3}, privateKey)
	require.Error(t, Store(path, ECDSA, []byte{4, 5, 6}))
}
