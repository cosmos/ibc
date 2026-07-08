package signer

import (
	"context"
	"fmt"
)

// KeyType key type supported by local Signer
type KeyType string

// Signer represents signer that can either sign digests or messages.
type Signer interface {
	Type() KeyType
	IsLocal() bool

	Sign(ctx context.Context, message []byte) ([]byte, error)
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
