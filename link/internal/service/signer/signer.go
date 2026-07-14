package signer

import (
	"context"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/service/signer/keyfile"
)

// KeyType key type supported by local Signer
type KeyType string

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
	EDDSA KeyType = KeyType(keyfile.EDDSA)
	ECDSA KeyType = KeyType(keyfile.ECDSA)
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

func NewSignerFromConfig(ctx context.Context, cfg config.SignerConfig) (signer Signer, alias string, err error) {
	switch cfg.Type {
	case config.SignerLocal:
		path, err := config.ExpandHome(cfg.File)
		if err != nil {
			return nil, "", errors.Wrap(err, "expand local signer file")
		}

		s, err := LocalKeyFromFile(path)

		return s, cfg.Alias, err
	case config.SignerRemote:
		s, err := NewRemoteFromURL(ctx, cfg.GRPC, cfg.RemoteKeyID)
		if err != nil {
			return nil, "", errors.Wrap(err, "create remote signer")
		}

		return s, cfg.Alias, err
	default:
		return nil, "", errors.Errorf("invalid signer type: %s", cfg.Type)
	}
}

func ParseKeyType(raw string) (KeyType, error) {
	parsed, err := keyfile.ParseType(raw)
	return KeyType(parsed), err
}
