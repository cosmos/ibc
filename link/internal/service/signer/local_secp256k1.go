package signer

import (
	"context"
	"crypto/rand"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"

	kms "github.com/cosmos/kms/signing/file"
)

// LocalSecp256k1Signer signs with a local secp256k1 key.
type LocalSecp256k1Signer struct {
	pk     *secp256k1.PrivateKey
	signer *kms.Secp256k1Signer
}

var _ LocalKey = (*LocalSecp256k1Signer)(nil)

func NewLocalSecp256k1Signer(privateKey []byte) (*LocalSecp256k1Signer, error) {
	var (
		pk     = secp256k1.PrivKeyFromBytes(privateKey)
		signer = kms.NewSecp256k1Signer(pk)
	)

	return &LocalSecp256k1Signer{pk: pk, signer: signer}, nil
}

func GenerateLocalSecp256k1Signer() (*LocalSecp256k1Signer, error) {
	pk, err := secp256k1.GeneratePrivateKeyFromRand(rand.Reader)
	if err != nil {
		return nil, err
	}

	return NewLocalSecp256k1Signer(pk.Serialize())
}

func (s *LocalSecp256k1Signer) Type() KeyType { return KeyECDSA }
func (s *LocalSecp256k1Signer) IsLocal() bool { return true }

func (s *LocalSecp256k1Signer) PubKey() []byte {
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
