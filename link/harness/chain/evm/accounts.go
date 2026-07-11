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

type Account struct {
	Key  *ecdsa.PrivateKey
	Addr common.Address
}

func FaucetAccount() Account {
	acct, err := AccountFromHex(testkeys.FaucetPrivateKeyHex)
	if err != nil {
		panic(fmt.Sprintf("invalid faucet key: %v", err))
	}
	return acct
}

func RelayerAccount() Account {
	acct, err := AccountFromHex(testkeys.RelayerPrivateKeyHex)
	if err != nil {
		panic(fmt.Sprintf("invalid relayer key: %v", err))
	}
	return acct
}

func AccountFromHex(hexKey string) (Account, error) {
	key, err := crypto.HexToECDSA(strings.TrimPrefix(hexKey, "0x"))
	if err != nil {
		return Account{}, fmt.Errorf("parse private key: %w", err)
	}
	return accountFromKey(key), nil
}

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

func signTx(tx *types.Transaction, key *ecdsa.PrivateKey, chainID *big.Int) (*types.Transaction, error) {
	signed, err := types.SignTx(tx, types.LatestSignerForChainID(chainID), key)
	if err != nil {
		return nil, fmt.Errorf("sign tx: %w", err)
	}
	return signed, nil
}
