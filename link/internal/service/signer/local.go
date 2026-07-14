package signer

import (
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/keyfile"
)

// LocalKey is a key stored and used locally.
type LocalKey interface {
	Signer
	PrivateKey() []byte
	StoreToFile(path string) error
}

func KeyFilePath(homePath, keyName string) (string, error) {
	filename := keyName + ".json"
	path := filepath.Join(homePath, "/keys", filepath.Clean(filename))
	return config.ExpandHome(path)
}

func GenerateLocalKey(keyType KeyType) (LocalKey, error) {
	switch keyType {
	case EDDSA:
		return GenerateLocalEd25519Signer()
	case ECDSA:
		return GenerateLocalSecp256k1Signer()
	default:
		return nil, errors.Errorf("invalid key type: %s", keyType)
	}
}

// LocalKeyFromFile loads a local key from the explicitly named path.
func LocalKeyFromFile(path string) (LocalKey, error) {
	keyType, privateKey, err := keyfile.Load(path)
	if err != nil {
		return nil, err
	}

	switch keyType {
	case EDDSA:
		return NewLocalEd25519Signer(privateKey)
	case ECDSA:
		return NewLocalSecp256k1Signer(privateKey)
	default:
		return nil, errors.Errorf("invalid key type: %s", keyType)
	}
}

func storeKeyToFile(path string, keyType KeyType, privateKey []byte) error {
	return keyfile.Store(path, keyType, privateKey)
}
