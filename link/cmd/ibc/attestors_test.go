package main

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/service/signer"
	"github.com/cosmos/ibc/link/keyfile"
)

// attestorFixture returns a config with one local ECDSA signer (alias
// "watcher-key"), an attestor "watcher-2" for chain 2 backed by it, and a
// remote signer alias "kms". Returns the config and the key's EVM address.
func attestorFixture(t *testing.T) (config.Config, string) {
	t.Helper()
	key, err := signer.GenerateLocalKey(keyfile.ECDSA)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "watcher-key.json")
	require.NoError(t, key.StoreToFile(path))
	address, err := signer.PublicKeyToEVMAddress(key.PublicKey())
	require.NoError(t, err)

	cfg := config.Config{
		Signers: config.Signers{
			{Alias: "watcher-key", Type: config.SignerLocal, File: path},
			{Alias: "kms", Type: config.SignerRemote, GRPC: "localhost:1", RemoteKeyID: "k"},
		},
		Attestors: config.Attestors{
			{ChainID: "2", Name: "watcher-2", Type: config.AttestorTypeLocal, Signer: "watcher-key"},
			{ChainID: "9", Name: "watcher-9", Type: config.AttestorTypeLocal, Signer: "kms"},
		},
	}
	return cfg, address
}

func TestResolveAttestorToken(t *testing.T) {
	cfg, address := attestorFixture(t)

	// anything that is not an alias passes through verbatim: address
	// formats are the target driver's to judge
	got, err := resolveAttestorToken(cfg, "0x00000000000000000000000000000000000000aa")
	require.NoError(t, err)
	require.Equal(t, "0x00000000000000000000000000000000000000aa", got)

	// attestor name resolves through its signer
	got, err = resolveAttestorToken(cfg, "watcher-2")
	require.NoError(t, err)
	require.Equal(t, address, got)

	// signer alias resolves directly
	got, err = resolveAttestorToken(cfg, "watcher-key")
	require.NoError(t, err)
	require.Equal(t, address, got)

	// an unknown token is treated as an address, not an error
	got, err = resolveAttestorToken(cfg, "nonsense")
	require.NoError(t, err)
	require.Equal(t, "nonsense", got)

	// a token that IS a known alias must resolve or fail — never fall
	// through to address passthrough
	_, err = resolveAttestorToken(cfg, "kms")
	require.ErrorContains(t, err, "remote signer")
}

func TestAttestorsForChain(t *testing.T) {
	cfg, address := attestorFixture(t)

	got, err := attestorsForChain(cfg, "2")
	require.NoError(t, err)
	require.Equal(t, []string{address}, got)

	// chain 9's only attestor resolves through a remote signer
	_, err = attestorsForChain(cfg, "9")
	require.ErrorContains(t, err, "remote signer")

	// nothing configured for chain 3
	_, err = attestorsForChain(cfg, "3")
	require.ErrorContains(t, err, "no attestors configured for chain 3")
}

func TestDefaultClientID(t *testing.T) {
	require.Equal(t, "link-31337-31338", defaultClientID("31338", "31337"))
	require.Equal(t, "link-31337-31338", defaultClientID("31337", "31338"))
}
