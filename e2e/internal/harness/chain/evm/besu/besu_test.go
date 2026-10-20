// SPDX-License-Identifier: Apache-2.0

package besu

import (
	"crypto/ecdsa"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
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
	require.Equal(t, validatorCount, cfg.Blockchain.Nodes.Count)
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

func TestPrepareBesuNodeDirs(t *testing.T) {
	t.Run("stages a data dir per validator", func(t *testing.T) {
		chainDir := t.TempDir()
		writeGeneratedBesuFile(t, chainDir, "genesis.json", "genesis")
		keys := writeGeneratedValidatorKeys(t, chainDir, validatorCount)

		validators, err := prepareBesuNodeDirs(chainDir, 0)
		require.NoError(t, err)
		require.FileExists(t, filepath.Join(chainDir, "genesis.json"))
		var want, got []common.Address
		for _, key := range keys {
			want = append(want, crypto.PubkeyToAddress(key.PublicKey))
		}
		for _, v := range validators {
			got = append(got, crypto.PubkeyToAddress(v.key.PublicKey))
			require.FileExists(t, filepath.Join(v.dataDir, "key"))
		}
		require.ElementsMatch(t, want, got)
	})

	t.Run("stages followers after the validators on new keys", func(t *testing.T) {
		chainDir := t.TempDir()
		writeGeneratedBesuFile(t, chainDir, "genesis.json", "genesis")
		keys := writeGeneratedValidatorKeys(t, chainDir, validatorCount)

		nodes, err := prepareBesuNodeDirs(chainDir, 2)
		require.NoError(t, err)
		require.Len(t, nodes, validatorCount+2)
		var validators []common.Address
		for _, key := range keys {
			validators = append(validators, crypto.PubkeyToAddress(key.PublicKey))
		}
		for i, n := range nodes {
			require.Equal(t, i < validatorCount, slices.Contains(validators, n.address()))
			require.FileExists(t, filepath.Join(n.dataDir, "key"))
		}
	})

	t.Run("rejects missing genesis", func(t *testing.T) {
		chainDir := t.TempDir()
		writeGeneratedValidatorKeys(t, chainDir, validatorCount)

		_, err := prepareBesuNodeDirs(chainDir, 0)
		require.ErrorContains(t, err, "copy besu genesis")
	})

	t.Run("rejects a wrong validator count", func(t *testing.T) {
		chainDir := t.TempDir()
		writeGeneratedBesuFile(t, chainDir, "genesis.json", "genesis")
		writeGeneratedValidatorKeys(t, chainDir, validatorCount-1)

		_, err := prepareBesuNodeDirs(chainDir, 0)
		require.ErrorContains(t, err, "validator key dirs")
	})

	t.Run("rejects missing validator key", func(t *testing.T) {
		chainDir := t.TempDir()
		writeGeneratedBesuFile(t, chainDir, "genesis.json", "genesis")
		writeGeneratedValidatorKeys(t, chainDir, validatorCount-1)
		require.NoError(t, os.MkdirAll(filepath.Join(chainDir, "networkFiles", "keys", "0xffff"), 0o755))

		_, err := prepareBesuNodeDirs(chainDir, 0)
		require.ErrorContains(t, err, "read validator key")
	})
}

// writeGeneratedValidatorKeys writes keys the way Besu's operator does: one
// dir per validator named by address, holding a 0x-prefixed hex key.
func writeGeneratedValidatorKeys(t *testing.T, chainDir string, n int) []*ecdsa.PrivateKey {
	t.Helper()
	keys := make([]*ecdsa.PrivateKey, n)
	for i := range keys {
		key, err := crypto.GenerateKey()
		require.NoError(t, err)
		keys[i] = key
		address := strings.ToLower(crypto.PubkeyToAddress(key.PublicKey).Hex())
		writeGeneratedBesuFile(
			t,
			chainDir,
			filepath.Join("keys", address, "key"),
			hexutil.Encode(crypto.FromECDSA(key)),
		)
	}
	return keys
}

func writeGeneratedBesuFile(t *testing.T, chainDir, relativePath, contents string) {
	t.Helper()
	path := filepath.Join(chainDir, "networkFiles", relativePath)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
}

// Peers dial a node by its public key, without the leading uncompressed-point
// byte, at its IP on the chain's network.
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
