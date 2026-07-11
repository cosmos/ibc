// Package evm contains shared ethclient-backed account, signing, and client helpers.
package evm

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/cosmos/ibc/link/harness/testkeys"
)

// Account is an EVM account the harness controls: a secp256k1 signing key plus its derived address.
type Account struct {
	Key  *ecdsa.PrivateKey
	Addr common.Address
}

// FaucetAccount returns the shared test faucet account.
func FaucetAccount() Account {
	acct, err := AccountFromHex(testkeys.FaucetPrivateKeyHex)
	if err != nil {
		// The faucet key is a compile-time constant; a parse failure is a programmer error.
		panic(fmt.Sprintf("invalid faucet key: %v", err))
	}
	return acct
}

// RelayerAccount returns the shared account used by the stub relay daemon.
func RelayerAccount() Account {
	acct, err := AccountFromHex(testkeys.RelayerPrivateKeyHex)
	if err != nil {
		// The relayer key is a compile-time constant; a parse failure is a programmer error.
		panic(fmt.Sprintf("invalid relayer key: %v", err))
	}
	return acct
}

// AccountFromHex parses a hex-encoded secp256k1 private key (with or without 0x) into an Account,
// deriving its address.
func AccountFromHex(hexKey string) (Account, error) {
	key, err := crypto.HexToECDSA(strings.TrimPrefix(hexKey, "0x"))
	if err != nil {
		return Account{}, fmt.Errorf("parse private key: %w", err)
	}
	return accountFromKey(key), nil
}

// NewAccount generates a fresh, unfunded account.
func NewAccount() (Account, error) {
	key, err := crypto.GenerateKey()
	if err != nil {
		return Account{}, fmt.Errorf("generate key: %w", err)
	}
	return accountFromKey(key), nil
}

func accountFromKey(key *ecdsa.PrivateKey) Account {
	return Account{Key: key, Addr: crypto.PubkeyToAddress(key.PublicKey)}
}

// signTx signs an EIP-1559 transaction with the latest signer for chainID.
func signTx(tx *types.Transaction, key *ecdsa.PrivateKey, chainID *big.Int) (*types.Transaction, error) {
	signed, err := types.SignTx(tx, types.LatestSignerForChainID(chainID), key)
	if err != nil {
		return nil, fmt.Errorf("sign tx: %w", err)
	}
	return signed, nil
}
