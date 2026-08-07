package signer

import (
	"context"
	"encoding/hex"
	"strings"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/keyfile"
)

// Signer represents signer that can either sign digests or messages.
type Signer interface {
	Type() keyfile.Type
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
	EDDSA = keyfile.EDDSA
	ECDSA = keyfile.ECDSA
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

		s, err := LocalKeyFromFile(config.KeyFileFallbacks(path)...)

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

func ParseKeyType(raw string) (keyfile.Type, error) {
	return keyfile.ParseType(raw)
}

func PublicKeyToEVMAddress(pk []byte) (string, error) {
	publicKey, err := crypto.DecompressPubkey(pk)
	if err != nil {
		return "", err
	}

	return crypto.PubkeyToAddress(*publicKey).Hex(), nil
}

func DecodeHex(raw string) ([]byte, error) {
	return hex.DecodeString(strings.TrimPrefix(raw, "0x"))
}

// EVMAddressOf derives the EVM address of a configured signer. Only local
// ECDSA signers resolve; remote signers would need a KMS round trip for
// their public key and are rejected.
func EVMAddressOf(cfg config.SignerConfig) (string, error) {
	if cfg.Type != config.SignerLocal {
		return "", errors.Errorf("cannot derive an address for remote signer %q", cfg.Alias)
	}

	path, err := config.ExpandHome(cfg.File)
	if err != nil {
		return "", err
	}

	key, err := LocalKeyFromFile(config.KeyFileFallbacks(path)...)
	if err != nil {
		return "", errors.Wrapf(err, "signer %q", cfg.Alias)
	}

	if key.Type() != keyfile.ECDSA {
		return "", errors.Errorf("signer %q is not an ecdsa key", cfg.Alias)
	}

	return PublicKeyToEVMAddress(key.PublicKey())
}
