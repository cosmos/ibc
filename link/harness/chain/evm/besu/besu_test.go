package besu

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/harness/chain/evm"
)

const expectedPrefundBalanceHex = "0xc9f2c9cd04674edea40000000"

func TestBesuOperatorConfigHarnessInvariants(t *testing.T) {
	const chainID = 32337
	const blockPeriodSecs = 2

	cfg := newBesuOperatorConfig(chainID, blockPeriodSecs, evm.RelayerAccount().Addr)

	require.Equal(t, uint64(chainID), cfg.Genesis.Config.ChainID)
	require.Equal(t, 1, cfg.Blockchain.Nodes.Count)
	require.Equal(t, blockPeriodSecs, cfg.Genesis.Config.QBFT.BlockPeriodSeconds)
	require.Equal(t, 2*blockPeriodSecs, cfg.Genesis.Config.QBFT.RequestTimeoutSeconds)
	require.True(t, cfg.Genesis.Config.ZeroBaseFee)
	require.Equal(t, uint64(0), cfg.Genesis.Config.LondonBlock)
	require.Equal(t, uint64(0), cfg.Genesis.Config.ShanghaiTime)

	faucet := besuAllocKey(evm.FaucetAccount().Addr)
	relayer := besuAllocKey(evm.RelayerAccount().Addr)
	require.Equal(t, expectedPrefundBalanceHex, cfg.Genesis.Alloc[faucet].Balance)
	require.Equal(t, expectedPrefundBalanceHex, cfg.Genesis.Alloc[relayer].Balance)
}

func TestPrepareBesuNodeDir(t *testing.T) {
	t.Run("stages complete generated output", func(t *testing.T) {
		chainDir := t.TempDir()
		writeGeneratedBesuFile(t, chainDir, "genesis.json", "genesis")
		writeGeneratedBesuFile(t, chainDir, filepath.Join("keys", "validator-1", "key"), "key")

		dataDir, err := prepareBesuNodeDir(chainDir)
		require.NoError(t, err)
		require.FileExists(t, filepath.Join(chainDir, "genesis.json"))
		require.FileExists(t, filepath.Join(dataDir, "key"))
	})

	t.Run("rejects missing genesis", func(t *testing.T) {
		chainDir := t.TempDir()
		writeGeneratedBesuFile(t, chainDir, filepath.Join("keys", "validator-1", "key"), "key")

		_, err := prepareBesuNodeDir(chainDir)
		require.ErrorContains(t, err, "copy besu genesis")
	})

	t.Run("rejects missing validator key", func(t *testing.T) {
		chainDir := t.TempDir()
		writeGeneratedBesuFile(t, chainDir, "genesis.json", "genesis")
		require.NoError(t, os.MkdirAll(filepath.Join(chainDir, "networkFiles", "keys", "validator-1"), 0o755))

		_, err := prepareBesuNodeDir(chainDir)
		require.ErrorContains(t, err, "copy validator key")
	})
}

func writeGeneratedBesuFile(t *testing.T, chainDir, relativePath, contents string) {
	t.Helper()
	path := filepath.Join(chainDir, "networkFiles", relativePath)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
}
