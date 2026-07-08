package signer

import (
	"fmt"
	"path/filepath"

	"github.com/cosmos/ibc/link/internal/config"
)

// KeyType key type supported by local Signer
type KeyType string

// KeyType enum
const (
	KeyEDDSA KeyType = "eddsa"
	KeyECDSA KeyType = "ecdsa"
)

var errInvalidKeyType = fmt.Errorf("key type must be one of [%s, %s]", KeyEDDSA, KeyECDSA)

func ParseKeyType(raw string) (KeyType, error) {
	if raw != string(KeyEDDSA) && raw != string(KeyECDSA) {
		return "", fmt.Errorf("%w: got %s", errInvalidKeyType, raw)
	}

	return KeyType(raw), nil
}

func KeyFilePath(homePath, file string) (string, error) {
	path := filepath.Join(homePath, "/keys", filepath.Clean(file))

	return config.ExpandHome(path)
}
