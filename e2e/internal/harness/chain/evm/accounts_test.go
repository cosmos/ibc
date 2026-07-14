package evm

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testPrivateKeyHex = "0000000000000000000000000000000000000000000000000000000000000001"

func TestAccountFromHexDerivesAddress(t *testing.T) {
	acct, err := AccountFromHex(testPrivateKeyHex)
	require.NoError(t, err)
	assert.Equal(t, common.HexToAddress("0x7E5F4552091A69125d5DfCb7b8C2659029395Bdf"), acct.Address())
}

func TestAccountFromHexAcceptsPrefix(t *testing.T) {
	withPrefix, err := AccountFromHex("0x" + testPrivateKeyHex)
	require.NoError(t, err)
	withoutPrefix, err := AccountFromHex(testPrivateKeyHex)
	require.NoError(t, err)
	assert.Equal(t, withoutPrefix.Address(), withPrefix.Address())
}

func TestSignTxRecoversSender(t *testing.T) {
	acct, err := AccountFromHex(testPrivateKeyHex)
	require.NoError(t, err)
	chainID := big.NewInt(31337)
	to := common.HexToAddress("0x0000000000000000000000000000000000000001")

	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID:   chainID,
		Nonce:     0,
		To:        &to,
		Value:     big.NewInt(1),
		Gas:       21000,
		GasFeeCap: big.NewInt(1_000_000_000),
		GasTipCap: big.NewInt(1_000_000_000),
	})

	opts, err := acct.TransactOpts(chainID)
	require.NoError(t, err)
	signed, err := opts.Signer(acct.Address(), tx)
	require.NoError(t, err)

	sender, err := types.Sender(types.LatestSignerForChainID(chainID), signed)
	require.NoError(t, err)
	assert.Equal(t, acct.Address(), sender)
}
