package stub

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/cmd/configcmd"

	internalconfig "github.com/cosmos/ibc/link/internal/config"
)

func TestLoadResolvesWholeEnvironmentReferences(t *testing.T) {
	t.Setenv("IBC_LINK_TEST_RPC_URL", "http://localhost:8545/?token=secret&literal=$X")
	t.Setenv("IBC_LINK_TEST_RELAYER_SIGNER", "/keys/relayer.json")
	t.Setenv("IBC_LINK_TEST_DB_TYPE", "sqlite")
	t.Setenv("IBC_LINK_TEST_DB_URL", "/data/relayer.db")
	path := writeConfig(t, `
chains:
  - id: chain-a
    type: evm
    chainId: 31337
    evmSigner: relayer
    testAppSigner: application
    rpc:
      url: ${IBC_LINK_TEST_RPC_URL}
signers:
  - alias: relayer
    type: local
    file: ${IBC_LINK_TEST_RELAYER_SIGNER}
  - alias: application
    type: local
    file: /keys/application.json
db:
  type: ${IBC_LINK_TEST_DB_TYPE}
  url: ${IBC_LINK_TEST_DB_URL}
relayer:
  routes: []
`)

	c, err := loadConfig(path)
	require.NoError(t, err)
	require.Equal(t, "http://localhost:8545/?token=secret&literal=$X", c.Chains[0].RPC.URL)
	require.Equal(t, relayerSignerAlias, c.Chains[0].EVMSigner)
	require.Equal(t, "application", c.Chains[0].TestAppSigner)
	require.Equal(
		t,
		[]string{relayerSignerAlias, "application"},
		[]string{c.Signers[0].Alias, c.Signers[1].Alias},
	)
	require.Equal(t, "/keys/relayer.json", c.Signers[0].File)
	require.Equal(t, "sqlite", c.DB.Type)
	require.Equal(t, "/data/relayer.db", c.DB.URL)
}

func TestLoadLeavesPartialURLReferenceLiteral(t *testing.T) {
	t.Setenv("IBC_LINK_TEST_RPC_TOKEN", "secret")
	path := writeConfig(t, `
chains:
  - id: chain-a
    type: evm
    chainId: 31337
    rpc:
      url: http://localhost:8545/?token=${IBC_LINK_TEST_RPC_TOKEN}
db:
  type: sqlite
  url: ibc-test.db
relayer:
  routes: []
`)

	c, err := loadConfig(path)
	require.NoError(t, err)
	require.Equal(t, "http://localhost:8545/?token=${IBC_LINK_TEST_RPC_TOKEN}", c.Chains[0].RPC.URL)
}

func TestLoadRejectsMissingURLReference(t *testing.T) {
	path := writeConfig(t, `
chains:
  - id: chain-a
    type: evm
    chainId: 31337
    rpc:
      url: ${IBC_LINK_TEST_MISSING_RPC}
db:
  type: sqlite
  url: ibc.db
relayer:
  routes: []
`)

	_, err := loadConfig(path)
	require.ErrorContains(t, err, "expand chains[0].rpc.url")
	require.ErrorContains(t, err, "environment variable IBC_LINK_TEST_MISSING_RPC is not set")
}

func TestLoadRejectsMissingSignerFileReference(t *testing.T) {
	path := writeConfig(t, `
signers:
  - alias: relayer
    type: local
    file: ${IBC_LINK_TEST_MISSING_SIGNER_FILE}
`)

	_, err := loadConfig(path)
	require.ErrorContains(t, err, "expand signers[0].file")
	require.ErrorContains(t, err, "environment variable IBC_LINK_TEST_MISSING_SIGNER_FILE is not set")
}

func TestLoadRejectsMissingDBReference(t *testing.T) {
	path := writeConfig(t, `
db:
  type: sqlite
  url: ${IBC_LINK_TEST_MISSING_DB_URL}
`)

	_, err := loadConfig(path)
	require.ErrorContains(t, err, "expand db.url")
	require.ErrorContains(t, err, "environment variable IBC_LINK_TEST_MISSING_DB_URL is not set")
}

func TestSetupUsesCommandConfigAndAppliesDBOverride(t *testing.T) {
	home := t.TempDir()
	t.Chdir(home)
	signerPath := filepath.Join(home, "not-created.json")
	t.Setenv("IBC_LINK_TEST_SIGNER_FILE", signerPath)
	require.NoError(t, os.WriteFile(filepath.Join(home, "ibc.yaml"), []byte(`
chains:
  - id: chain-a
    type: evm
    chainId: 31337
    evmSigner: relayer
    rpc:
      url: http://localhost:8545
signers:
  - alias: relayer
    type: local
    file: ${IBC_LINK_TEST_SIGNER_FILE}
db:
  type: sqlite
  url: relayer.db
`), 0o600))

	flags := internalconfig.DefaultFlagSet()
	flags.Home = home
	flags.Config = "ibc.yaml"
	flags.DB = filepath.Join(home, "override.db")
	c, err := setupConfig(&flags)
	require.NoError(t, err)
	require.Equal(t, "chain-a", c.Chains[0].ID)
	require.Equal(t, signerPath, c.Signers[0].File)
	require.Equal(t, "sqlite", c.DB.Type)
	require.Equal(t, flags.DB, c.DB.URL)
	cwd, err := os.Getwd()
	require.NoError(t, err)
	require.Equal(t, home, cwd)
}

func TestDBConfigFromPath(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{name: "sqlite path", raw: "relayer.db"},
		{name: "empty", wantErr: "db url is empty"},
		{name: "in-memory sqlite", raw: ":memory:", wantErr: "in-memory sqlite"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, err := dbConfigFromPath(test.raw)
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, configcmd.DBTypeSQLite, db.Type)
			require.Equal(t, test.raw, db.URL)
		})
	}
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ibc.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}
