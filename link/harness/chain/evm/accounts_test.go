package evm

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/harness/testkeys"
)

// The faucet key/address pair is load-bearing: every funded account is paid from it, so a wrong
// derivation would break the whole provider. Assert the constant address is exactly what the key
// derives to.
func TestFaucetAccountDerivation(t *testing.T) {
	acct := FaucetAccount()
	require.NotNil(t, acct.Key)
	assert.Equal(t, common.HexToAddress(testkeys.FaucetAddressHex), acct.Addr)
}

func TestRelayerAccountDerivation(t *testing.T) {
	acct := RelayerAccount()
	require.NotNil(t, acct.Key)
	assert.Equal(t, common.HexToAddress(testkeys.RelayerAddressHex), acct.Addr)
}

func TestAccountFromHexAcceptsPrefix(t *testing.T) {
	withPrefix, err := AccountFromHex("0x" + testkeys.FaucetPrivateKeyHex)
	require.NoError(t, err)
	withoutPrefix, err := AccountFromHex(testkeys.FaucetPrivateKeyHex)
	require.NoError(t, err)
	assert.Equal(t, withoutPrefix.Addr, withPrefix.Addr)
}

// signTx must produce a signature that recovers back to the signer's address, proving the helper
// uses the right signer for the chain id.
func TestSignTxRecoversSender(t *testing.T) {
	acct := FaucetAccount()
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

	signed, err := signTx(tx, acct.Key, chainID)
	require.NoError(t, err)

	sender, err := types.Sender(types.LatestSignerForChainID(chainID), signed)
	require.NoError(t, err)
	assert.Equal(t, acct.Addr, sender)
}
