package cfg

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadExpandsURLReferences(t *testing.T) {
	t.Setenv("IBC_LINK_TEST_RPC_TOKEN", "secret")
	path := writeConfig(t, `
chains:
  - id: chain-a
    type: evm
    provider: anvil
    chainId: 31337
    rpc:
      url: http://localhost:8545/?token=${IBC_LINK_TEST_RPC_TOKEN}&literal=$X
db:
  type: sqlite
  url: ibc-test.db
relayer:
  routes: []
`)

	c, err := Load(path)
	require.NoError(t, err)
	require.Equal(t, "http://localhost:8545/?token=secret&literal=$X", c.Chains[0].RPC.URL)
}

func TestLoadRejectsMissingURLReference(t *testing.T) {
	path := writeConfig(t, `
chains:
  - id: chain-a
    type: evm
    provider: anvil
    chainId: 31337
    rpc:
      url: ${IBC_LINK_TEST_MISSING_RPC}
db:
  type: sqlite
  url: ibc.db
relayer:
  routes: []
`)

	_, err := Load(path)
	require.ErrorContains(t, err, "expand chains[0].rpc.url")
	require.ErrorContains(t, err, "environment variable IBC_LINK_TEST_MISSING_RPC is not set")
}

func TestLoadExpandsCosmosGRPCReference(t *testing.T) {
	t.Setenv("IBC_LINK_TEST_GRPC", "localhost:9090")
	path := writeConfig(t, `
chains:
  - id: chain-b
    type: cosmos
    provider: sandbox
    cosmosChainId: sandbox-cosmos-1
    grpcUrl: ${IBC_LINK_TEST_GRPC}
    signerKey: signer
    faucetKey: faucet
    rpc:
      url: http://localhost:26657
db:
  type: sqlite
  url: ibc.db
relayer:
  routes: []
`)

	c, err := Load(path)
	require.NoError(t, err)
	require.Equal(t, "localhost:9090", c.Chains[0].GRPCURL)
}

func TestLoadRejectsMissingCosmosGRPCReference(t *testing.T) {
	path := writeConfig(t, `
chains:
  - id: chain-b
    type: cosmos
    provider: sandbox
    cosmosChainId: sandbox-cosmos-1
    grpcUrl: ${IBC_LINK_TEST_MISSING_GRPC}
    signerKey: signer
    faucetKey: faucet
    rpc:
      url: http://localhost:26657
db:
  type: sqlite
  url: ibc.db
relayer:
  routes: []
`)

	_, err := Load(path)
	require.ErrorContains(t, err, "expand chains[0].grpcUrl")
	require.ErrorContains(t, err, "environment variable IBC_LINK_TEST_MISSING_GRPC is not set")
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ibc.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}
