// SPDX-License-Identifier: Apache-2.0

package signer

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/fsutil"
	"github.com/cosmos/ibc/link/keyfile"
)

// LocalKey is a key stored and used locally.
type LocalKey interface {
	Signer
	PrivateKey() []byte
	StoreToFile(path string) error
}

// PersistedLocalKey is a local key loaded from disk.
type PersistedLocalKey struct {
	LocalKey
	Path string
}

func KeyFilePath(homePath, keyName string) (string, error) {
	filename := keyName + ".json"
	path := filepath.Join(homePath, "/keys", filepath.Clean(filename))
	return fsutil.ExpandHome(path)
}

func GenerateLocalKey(keyType keyfile.Type) (LocalKey, error) {
	switch keyType {
	case EDDSA:
		return GenerateLocalEd25519Signer()
	case ECDSA:
		return GenerateLocalSecp256k1Signer()
	default:
		return nil, errors.Errorf("invalid key type: %s", keyType)
	}
}

// LocalKeyFromFile loads a local key from the first path that resolves.
func LocalKeyFromFile(path ...string) (LocalKey, error) {
	var (
		err error
		key LocalKey
	)

	for _, tryPath := range path {
		key, err = localKeyFromFile(tryPath)
		if err == nil {
			return key, nil
		}
	}

	return nil, err
}

func LocalKeysFromDirectory(keysDirectory string) ([]PersistedLocalKey, error) {
	keys := []PersistedLocalKey{}

	entries, err := os.ReadDir(keysDirectory)
	if err != nil {
		if os.IsNotExist(err) {
			return keys, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		path := filepath.Join(keysDirectory, entry.Name())
		key, err := LocalKeyFromFile(path)
		if err != nil {
			continue
		}

		keys = append(keys, PersistedLocalKey{
			LocalKey: key,
			Path:     path,
		})
	}

	return keys, nil
}

func localKeyFromFile(path string) (LocalKey, error) {
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

func storeKeyToFile(path string, keyType keyfile.Type, privateKey []byte) error {
	return keyfile.Store(path, keyType, privateKey)
}

func (k *PersistedLocalKey) Name() string {
	return strings.TrimSuffix(filepath.Base(k.Path), ".json")
}
