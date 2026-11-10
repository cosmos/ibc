// SPDX-License-Identifier: Apache-2.0

package signer

import (
	"context"
	"math/big"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"

	"github.com/lfdt-ibc/ibc/cli/internal/config"
	"github.com/lfdt-ibc/ibc/cli/keyfile"
)

func TestNewEVMTransactor(t *testing.T) {
	ctx := context.Background()
	chainID := big.NewInt(1337)

	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	s, err := NewLocalSecp256k1Signer(crypto.FromECDSA(key))
	require.NoError(t, err)
	address := crypto.PubkeyToAddress(key.PublicKey)

	opts, err := NewEVMTransactor(ctx, s, chainID)
	require.NoError(t, err)

	assertSignsAs(t, opts.Signer, address, chainID)

	// apart from the signer, the same opts a raw-key transactor builds, so
	// gas and fees are estimated the same way
	keyed, err := bind.NewKeyedTransactorWithChainID(key, chainID)
	require.NoError(t, err)
	keyed.Context = ctx
	keyed.Signer, opts.Signer = nil, nil
	require.Equal(t, keyed, opts)

	_, err = NewEVMTransactor(ctx, s, nil)
	require.ErrorIs(t, err, bind.ErrNoChainID)

	ed, err := GenerateLocalEd25519Signer()
	require.NoError(t, err)
	_, err = NewEVMTransactor(ctx, ed, chainID)
	require.ErrorContains(t, err, "ECDSA")
}

// TestNewEVMTransactorFromConfig signs through the signer a deploy command
// builds from config, metrics wrapper included, without any otel setup.
func TestNewEVMTransactorFromConfig(t *testing.T) {
	ctx := context.Background()
	chainID := big.NewInt(1337)

	key, err := GenerateLocalKey(keyfile.ECDSA)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "deployer.json")
	require.NoError(t, key.StoreToFile(path))
	address, err := PublicKeyToEVMAddress(key.PublicKey())
	require.NoError(t, err)

	s, _, err := NewSignerFromConfig(ctx, config.SignerConfig{Alias: "deployer", Type: config.SignerLocal, File: path})
	require.NoError(t, err)

	opts, err := NewEVMTransactor(ctx, s, chainID)
	require.NoError(t, err)
	assertSignsAs(t, opts.Signer, common.HexToAddress(address), chainID)
}

// assertSignsAs checks sign signs as address and refuses any other sender.
func assertSignsAs(t *testing.T, sign bind.SignerFn, address common.Address, chainID *big.Int) {
	t.Helper()

	tx := types.NewTx(&types.DynamicFeeTx{ChainID: chainID, Nonce: 1, Gas: 21_000})

	signed, err := sign(address, tx)
	require.NoError(t, err)
	sender, err := types.Sender(types.LatestSignerForChainID(chainID), signed)
	require.NoError(t, err)
	require.Equal(t, address, sender)

	_, err = sign(common.HexToAddress("0x01"), tx)
	require.ErrorIs(t, err, bind.ErrNotAuthorized)
}
