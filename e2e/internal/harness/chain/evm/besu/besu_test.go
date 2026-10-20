// SPDX-License-Identifier: Apache-2.0

package besu

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"

	clibesu "github.com/cosmos/ibc/cli/besu"
)

const expectedTreasuryBalanceHex = "0xc9f2c9cd04674edea40000000"

func TestBesuOperatorConfigEnvironmentInvariants(t *testing.T) {
	const chainID = 32337
	const blockPeriodSecs = 2

	treasury := common.HexToAddress("0x1000000000000000000000000000000000000001")
	cfg := newBesuOperatorConfig(clibesu.QBFT, chainID, blockPeriodSecs, treasury)

	require.Equal(t, uint64(chainID), cfg.Genesis.Config.ChainID)
	require.Equal(t, 1, cfg.Blockchain.Nodes.Count)
	require.Nil(t, cfg.Genesis.Config.IBFT2)
	require.Equal(t, blockPeriodSecs, cfg.Genesis.Config.QBFT.BlockPeriodSeconds)
	require.Equal(t, 2*blockPeriodSecs, cfg.Genesis.Config.QBFT.RequestTimeoutSeconds)
	require.True(t, cfg.Genesis.Config.ZeroBaseFee)
	require.Equal(t, uint64(0), cfg.Genesis.Config.LondonBlock)
	require.Equal(t, uint64(0), cfg.Genesis.Config.ShanghaiTime)
	require.Equal(t, uint64(0), cfg.Genesis.Config.CancunTime)

	require.Equal(t, map[string]besuFund{
		besuAllocKey(treasury): {Balance: expectedTreasuryBalanceHex},
	}, cfg.Genesis.Alloc)
}

// Besu reads the consensus from the genesis config key, so an IBFT2 chain
// carries an ibft2 block in place of the qbft one.
func TestBesuOperatorConfigIBFT2(t *testing.T) {
	cfg := newBesuOperatorConfig(clibesu.IBFT2, 32337, 2, common.Address{})
	require.Nil(t, cfg.Genesis.Config.QBFT)
	require.Equal(t, 2, cfg.Genesis.Config.IBFT2.BlockPeriodSeconds)

	data, err := json.Marshal(cfg.Genesis.Config)
	require.NoError(t, err)
	var keys map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &keys))
	require.Contains(t, keys, "ibft2")
	require.NotContains(t, keys, "qbft")
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

// Followers dial the validator node by its public key, without the leading
// uncompressed-point byte, at its bridge IP.
func TestEnode(t *testing.T) {
	key, err := crypto.HexToECDSA("0000000000000000000000000000000000000000000000000000000000000001")
	require.NoError(t, err)
	require.Equal(t,
		"enode://79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798"+
			"483ada7726a3c4655da4fbfc0e1108a8fd17b448a68554199c47d08ffb10d4b8@172.17.0.2:30303",
		enode(key, "172.17.0.2"))
}

func TestVotingAPI(t *testing.T) {
	require.Equal(t, "qbft", votingAPI(clibesu.QBFT))
	require.Equal(t, "ibft", votingAPI(clibesu.IBFT2))
}
