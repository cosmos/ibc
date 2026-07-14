package signing

import (
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/harness/ibclink/wire"
	"github.com/cosmos/ibc/e2e/internal/keyfile"
)

func TestLoadECDSA(t *testing.T) {
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "relayer.json")
	require.NoError(t, keyfile.Store(path, keyfile.ECDSA, crypto.FromECDSA(key)))

	got, err := LoadECDSA([]wire.Signer{{
		Alias: "relayer",
		Type:  wire.SignerTypeLocal,
		File:  path,
	}}, "relayer")
	require.NoError(t, err)
	require.Equal(t, crypto.FromECDSA(key), crypto.FromECDSA(got))
}

func TestLoadECDSARejectsUnknownAlias(t *testing.T) {
	_, err := LoadECDSA(nil, "missing")
	require.ErrorContains(t, err, `signer alias "missing" is not configured`)
}

func TestLoadECDSARejectsNonECDSAKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "application.json")
	require.NoError(t, keyfile.Store(path, keyfile.EDDSA, make([]byte, 64)))

	_, err := LoadECDSA([]wire.Signer{{
		Alias: "application",
		Type:  wire.SignerTypeLocal,
		File:  path,
	}}, "application")
	require.ErrorContains(t, err, `local signer "application" must contain an ECDSA key`)
}

func TestLoadECDSAUsesExactConfiguredPath(t *testing.T) {
	dir := t.TempDir()
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	require.NoError(t, keyfile.Store(
		filepath.Join(dir, "relayer.json"),
		keyfile.ECDSA,
		crypto.FromECDSA(key),
	))

	configuredPath := filepath.Join(dir, "relayer")
	_, err = LoadECDSA([]wire.Signer{{
		Alias: "relayer",
		Type:  wire.SignerTypeLocal,
		File:  configuredPath,
	}}, "relayer")
	require.Error(t, err)
	require.ErrorContains(t, err, configuredPath)
}
