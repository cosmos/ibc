// SPDX-License-Identifier: Apache-2.0

package signer

import (
	"context"
	"crypto/rand"

	kms "github.com/cosmos/kms/signing/file"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"

	"github.com/cosmos/ibc/link/keyfile"
)

// LocalSecp256k1Signer signs with a local secp256k1 key.
type LocalSecp256k1Signer struct {
	pk     *secp256k1.PrivateKey
	signer *kms.Secp256k1EthSigner
}

var _ LocalKey = (*LocalSecp256k1Signer)(nil)

func NewLocalSecp256k1Signer(privateKey []byte) (*LocalSecp256k1Signer, error) {
	pk := secp256k1.PrivKeyFromBytes(privateKey)

	signer, err := kms.NewSecp256k1Eth(privateKey)
	if err != nil {
		return nil, err
	}

	return &LocalSecp256k1Signer{pk: pk, signer: signer}, nil
}

func GenerateLocalSecp256k1Signer() (*LocalSecp256k1Signer, error) {
	pk, err := secp256k1.GeneratePrivateKeyFromRand(rand.Reader)
	if err != nil {
		return nil, err
	}

	return NewLocalSecp256k1Signer(pk.Serialize())
}

func (s *LocalSecp256k1Signer) Type() keyfile.Type { return ECDSA }
func (s *LocalSecp256k1Signer) IsLocal() bool      { return true }

func (s *LocalSecp256k1Signer) PublicKey() []byte {
	return s.signer.PubKey()
}

func (s *LocalSecp256k1Signer) PrivateKey() []byte {
	return s.pk.Serialize()
}

// Sign signs ECDSA *digest* (not message)
func (s *LocalSecp256k1Signer) Sign(ctx context.Context, digest []byte) ([]byte, error) {
	return s.signer.Sign(ctx, digest)
}

func (s *LocalSecp256k1Signer) StoreToFile(path string) error {
	return storeKeyToFile(path, s.Type(), s.pk.Serialize())
}
