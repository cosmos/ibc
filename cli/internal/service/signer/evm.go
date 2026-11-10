// SPDX-License-Identifier: Apache-2.0

package signer

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/pkg/errors"
)

// EVMAddress derives the EVM address of an ECDSA signer.
func EVMAddress(s Signer) (common.Address, error) {
	if s.Type() != ECDSA {
		return common.Address{}, errors.New("EVM transactions require an ECDSA signer")
	}

	pub, err := crypto.DecompressPubkey(s.PublicKey())
	if err != nil {
		return common.Address{}, errors.Wrap(err, "decompressing signer public key")
	}

	return crypto.PubkeyToAddress(*pub), nil
}

// SignEVMTx signs tx with s, whose EVM address is address, and checks the
// signature recovers to that address.
func SignEVMTx(
	ctx context.Context,
	tx *types.Transaction,
	s Signer,
	ethSigner types.Signer,
	address common.Address,
) (*types.Transaction, error) {
	sig, err := s.Sign(ctx, ethSigner.Hash(tx).Bytes())
	if err != nil {
		return nil, errors.Wrapf(err, "signing transaction with address %s", address)
	}
	signed, err := tx.WithSignature(ethSigner, sig)
	if err != nil {
		return nil, errors.Wrap(err, "attaching transaction signature")
	}
	sender, err := types.Sender(ethSigner, signed)
	if err != nil {
		return nil, errors.Wrap(err, "recovering transaction sender")
	}
	if sender != address {
		return nil, errors.New("transaction signature does not match configured sender")
	}
	return signed, nil
}

// NewEVMTransactor the signer-backed equivalent of
// bind.NewKeyedTransactorWithChainID: it sets only From, Signer, and Context,
// so the bindings estimate gas and fees exactly as for a raw key.
func NewEVMTransactor(ctx context.Context, s Signer, chainID *big.Int) (*bind.TransactOpts, error) {
	if chainID == nil {
		return nil, bind.ErrNoChainID
	}

	address, err := EVMAddress(s)
	if err != nil {
		return nil, err
	}

	ethSigner := types.LatestSignerForChainID(chainID)

	return &bind.TransactOpts{
		From: address,
		Signer: func(from common.Address, tx *types.Transaction) (*types.Transaction, error) {
			if from != address {
				return nil, bind.ErrNotAuthorized
			}
			return SignEVMTx(ctx, tx, s, ethSigner, address)
		},
		Context: ctx,
	}, nil
}
