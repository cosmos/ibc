// Package evm contains shared ethclient-backed account, signing, and client helpers.
package evm

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

type Account struct {
	key  *ecdsa.PrivateKey
	addr common.Address
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
	return Account{key: key, addr: crypto.PubkeyToAddress(key.PublicKey)}
}

func (a Account) Address() common.Address { return a.addr }

func (a Account) TransactOpts(chainID *big.Int) (*bind.TransactOpts, error) {
	if a.key == nil {
		return nil, fmt.Errorf("account has no private key")
	}
	opts, err := bind.NewKeyedTransactorWithChainID(a.key, chainID)
	if err != nil {
		return nil, fmt.Errorf("create transactor: %w", err)
	}
	return opts, nil
}

func signTx(tx *types.Transaction, key *ecdsa.PrivateKey, chainID *big.Int) (*types.Transaction, error) {
	signed, err := types.SignTx(tx, types.LatestSignerForChainID(chainID), key)
	if err != nil {
		return nil, fmt.Errorf("sign tx: %w", err)
	}
	return signed, nil
}
