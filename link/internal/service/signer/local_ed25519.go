package signer

import (
	"context"
	"crypto/rand"

	"github.com/cometbft/cometbft/crypto"
	"github.com/cometbft/cometbft/crypto/ed25519"

	kms "github.com/cosmos/kms/signing/file"

	"github.com/cosmos/ibc/link/keyfile"
)

// LocalEd25519Signer signs with a local Ed25519 key.
type LocalEd25519Signer struct {
	pk     crypto.PrivKey
	signer *kms.Ed25519Signer
}

var _ LocalKey = (*LocalEd25519Signer)(nil)

func NewLocalEd25519Signer(privateKey []byte) (*LocalEd25519Signer, error) {
	signer, err := kms.NewEd25519(privateKey)
	if err != nil {
		return nil, err
	}

	pk := ed25519.PrivKey(privateKey)

	return &LocalEd25519Signer{pk: pk, signer: signer}, nil
}

func GenerateLocalEd25519Signer() (*LocalEd25519Signer, error) {
	kmsSigner, err := kms.GenerateEd25519(rand.Reader)
	if err != nil {
		return nil, err
	}

	priv, err := kms.PrivateKeyFromSigner(kmsSigner)
	if err != nil {
		return nil, err
	}

	pk := ed25519.PrivKey(priv)

	return &LocalEd25519Signer{pk: pk, signer: kmsSigner}, nil
}

func (s *LocalEd25519Signer) IsLocal() bool      { return true }
func (s *LocalEd25519Signer) Type() keyfile.Type { return EDDSA }

func (s *LocalEd25519Signer) PublicKey() []byte {
	return s.signer.PubKey()
}

func (s *LocalEd25519Signer) PrivateKey() []byte {
	return s.pk.Bytes()
}

// Sign signs the provided message with Ed25519
func (s *LocalEd25519Signer) Sign(ctx context.Context, message []byte) ([]byte, error) {
	return s.signer.Sign(ctx, message)
}

func (s *LocalEd25519Signer) StoreToFile(path string) error {
	return storeKeyToFile(path, s.Type(), s.PrivateKey())
}
