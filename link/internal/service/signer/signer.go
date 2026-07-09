package signer

import (
	"context"
	"fmt"

	"github.com/cosmos/ibc/link/internal/config"
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
	PublicKey() []byte
}

// Set a set of Signer accessible by their alias.
type Set struct {
	set map[string]Signer
}

// KeyType & SignerType enums
const (
	EDDSA  KeyType = "eddsa"
	ECDSA  KeyType = "ecdsa"
	Local  Source  = "local"
	Remote Source  = "remote"
)

func NewSet() *Set {
	return &Set{
		set: make(map[string]Signer),
	}
}

func (s *Set) Set(alias string, signer Signer) {
	s.set[alias] = signer
}

func (s *Set) Get(alias string) (Signer, bool) {
	signer, ok := s.set[alias]
	return signer, ok
}

func NewSignerFromConfig(config config.SignerConfig) (signer Signer, alias string, err error) {
	switch Source(config.Type) {
	case Local:
		s, err := LocalKeyFromFile(config.File)

		return s, config.Alias, err
	case Remote:
		return nil, "", fmt.Errorf("remote signers are not supported yet")
	default:
		return nil, "", fmt.Errorf("invalid signer type: %s", config.Type)
	}
}

func ParseKeyType(raw string) (KeyType, error) {
	if raw != string(EDDSA) && raw != string(ECDSA) {
		return "", fmt.Errorf("invalid key type: %s", raw)
	}

	return KeyType(raw), nil
}
