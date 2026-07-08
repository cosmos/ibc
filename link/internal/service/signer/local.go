package signer

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cosmos/ibc/link/internal/config"
)

// KeyType key type supported by local Signer
type KeyType string

type Key interface {
	Type() KeyType
	IsLocal() bool
}

type LocalKey interface {
	Key
	PubKey() []byte
	PrivateKey() []byte
	StoreToFile(path string) error
}

type keyJSON struct {
	Type       KeyType `json:"type"`
	PrivateKey string  `json:"privateKeyBase64"`
}

// KeyType enum
const (
	KeyEDDSA KeyType = "eddsa"
	KeyECDSA KeyType = "ecdsa"
)

func ParseKeyType(raw string) (KeyType, error) {
	if raw != string(KeyEDDSA) && raw != string(KeyECDSA) {
		return "", fmt.Errorf("invalid key type: %s", raw)
	}

	return KeyType(raw), nil
}

func KeyFilePath(homePath, keyName string) (string, error) {
	filename := fmt.Sprintf("%s.json", keyName)
	path := filepath.Join(homePath, "/keys", filepath.Clean(filename))
	return config.ExpandHome(path)
}

func GenerateLocalKey(keyType KeyType) (LocalKey, error) {
	switch keyType {
	case KeyEDDSA:
		return GenerateLocalEd25519Signer()
	case KeyECDSA:
		return GenerateLocalSecp256k1Signer()
	default:
		return nil, fmt.Errorf("invalid key type: %s", keyType)
	}
}

func LocalKeyFromFile(path string) (LocalKey, error) {
	keyType, privateKey, err := loadKeyFromFile(path)
	if err != nil {
		return nil, err
	}

	switch keyType {
	case KeyEDDSA:
		return NewLocalEd25519Signer(privateKey)
	case KeyECDSA:
		return NewLocalSecp256k1Signer(privateKey)
	default:
		return nil, fmt.Errorf("invalid key type: %s", keyType)
	}
}

func storeKeyToFile(path string, keyType KeyType, privateKey []byte) error {
	data := keyJSON{
		Type:       keyType,
		PrivateKey: base64.StdEncoding.EncodeToString(privateKey),
	}

	bz, err := json.Marshal(data)
	if err != nil {
		return err
	}

	if err := config.EnsureDirectory(path); err != nil {
		return err
	}

	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("file %s already exists", path)
	}

	return os.WriteFile(path, bz, 0o644)
}

func loadKeyFromFile(path string) (KeyType, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}

	var key keyJSON
	if err = json.Unmarshal(data, &key); err != nil {
		return "", nil, err
	}

	_, err = ParseKeyType(string(key.Type))
	if err != nil {
		return "", nil, err
	}

	privateKey, err := base64.StdEncoding.DecodeString(key.PrivateKey)
	if err != nil {
		return "", nil, err
	}

	return key.Type, privateKey, nil
}
