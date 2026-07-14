package stub

import (
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/cmd/configcmd"
	"github.com/cosmos/ibc/link/keyfile"
)

func TestLoadECDSA(t *testing.T) {
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "relayer.json")
	require.NoError(t, keyfile.Store(path, keyfile.ECDSA, crypto.FromECDSA(key)))

	got, err := loadECDSA([]configcmd.Signer{{
		Alias: relayerSignerAlias,
		Type:  configcmd.SignerTypeLocal,
		File:  path,
	}}, relayerSignerAlias)
	require.NoError(t, err)
	require.Equal(t, crypto.FromECDSA(key), crypto.FromECDSA(got))
}

func TestLoadECDSARejectsUnknownAlias(t *testing.T) {
	_, err := loadECDSA(nil, "missing")
	require.ErrorContains(t, err, `signer alias "missing" is not configured`)
}

func TestLoadECDSARejectsNonECDSAKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "application.json")
	require.NoError(t, keyfile.Store(path, keyfile.EDDSA, make([]byte, 64)))

	_, err := loadECDSA([]configcmd.Signer{{
		Alias: "application",
		Type:  configcmd.SignerTypeLocal,
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
	_, err = loadECDSA([]configcmd.Signer{{
		Alias: relayerSignerAlias,
		Type:  configcmd.SignerTypeLocal,
		File:  configuredPath,
	}}, relayerSignerAlias)
	require.Error(t, err)
	require.ErrorContains(t, err, configuredPath)
}
