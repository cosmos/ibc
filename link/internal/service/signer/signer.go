package signer

import (
	"context"
	"fmt"
)

// KeyType key type supported by local Signer
type KeyType string

// Source signer source supported by the system
type Source string

// Signer represents signer that can either sign digests or messages.
type Signer interface {
	Type() KeyType
	IsLocal() bool

	Sign(ctx context.Context, message []byte) ([]byte, error)
}

// KeyType & SignerType enums
const (
	EDDSA  KeyType = "eddsa"
	ECDSA  KeyType = "ecdsa"
	Local  Source  = "local"
	Remote Source  = "remote"
)

func ParseKeyType(raw string) (KeyType, error) {
	if raw != string(EDDSA) && raw != string(ECDSA) {
		return "", fmt.Errorf("invalid key type: %s", raw)
	}

	return KeyType(raw), nil
}
