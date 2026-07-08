package signer

import (
	"context"

	"github.com/cometbft/cometbft/crypto"
	"github.com/cometbft/cometbft/crypto/ed25519"

	kms "github.com/cosmos/kms/signing/file"
)

// LocalEd25519Signer signs with a local Ed25519 key.
type LocalEd25519Signer struct {
	pk      crypto.PrivKey
	backend *kms.Backend
}

var _ LocalKey = (*LocalEd25519Signer)(nil)

func NewLocalEd25519Signer(privateKey []byte) (*LocalEd25519Signer, error) {
	var (
		pk      = ed25519.PrivKey(privateKey)
		backend = kms.NewEd25519(pk)
	)

	return &LocalEd25519Signer{pk: pk, backend: backend}, nil
}

func GenerateLocalEd25519Signer() (*LocalEd25519Signer, error) {
	priv := ed25519.GenPrivKey()

	return NewLocalEd25519Signer(priv)
}

func (s *LocalEd25519Signer) IsLocal() bool { return true }
func (s *LocalEd25519Signer) Type() KeyType { return KeyEDDSA }

func (s *LocalEd25519Signer) PubKey() []byte {
	pub, err := s.backend.PubKey(context.Background())
	if err != nil {
		// local key can't error (check sources)
		panic(err)
	}

	return pub.Bytes()
}

func (s *LocalEd25519Signer) PrivateKey() []byte {
	return s.pk.Bytes()
}

// Sign signs EDDSA message or digest (based on the message length)
func (s *LocalEd25519Signer) Sign(ctx context.Context, message []byte) ([]byte, error) {
	return s.backend.Sign(ctx, message)
}

func (s *LocalEd25519Signer) StoreToFile(path string) error {
	return storeKeyToFile(path, s.Type(), s.PrivateKey())
}
