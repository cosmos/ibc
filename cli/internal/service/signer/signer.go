// SPDX-License-Identifier: Apache-2.0

package signer

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"strings"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/cli/internal/config"
	"github.com/cosmos/ibc/cli/keyfile"
)

// Signer represents signer that can either sign digests or messages.
type Signer interface {
	Type() keyfile.Type

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

func NewSetFromConfig(ctx context.Context, signers config.Signers) (*Set, error) {
	set := NewSet()

	for _, signerConfig := range signers {
		s, alias, err := NewSignerFromConfig(ctx, signerConfig)
		if err != nil {
			return nil, err
		}

		set.Set(alias, s)
	}

	return set, nil
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
		if err != nil {
			return nil, "", err
		}

		return metricsWrapper(s, cfg.Alias, localType(s.Type())), cfg.Alias, nil
	case config.SignerRemote:
		// Not logging cfg.GRPC: a target can carry userinfo.
		endpoint, err := config.ResolveEndpoint(cfg.GRPC, cfg.TLS, nil, "signer", cfg.Alias)
		if err != nil {
			return nil, "", errors.Wrapf(err, "signer %q", cfg.Alias)
		}

		s, err := NewRemoteFromEndpoint(ctx, cfg.RemoteKeyID, endpoint)
		if err != nil {
			return nil, "", errors.Wrap(err, "create remote signer")
		}

		return metricsWrapper(s, cfg.Alias, typeRemote), cfg.Alias, nil
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

// LocalECDSAKey loads the private key of a configured local ECDSA signer, for
// tooling that signs with the raw key. Remote signers are rejected: their key
// never leaves the KMS.
func LocalECDSAKey(cfg config.SignerConfig) (*ecdsa.PrivateKey, error) {
	if cfg.Type != config.SignerLocal {
		return nil, errors.Errorf("remote signer %q has no local key", cfg.Alias)
	}

	path, err := config.ExpandHome(cfg.File)
	if err != nil {
		return nil, err
	}

	key, err := LocalKeyFromFile(config.KeyFileFallbacks(path)...)
	if err != nil {
		return nil, errors.Wrapf(err, "signer %q", cfg.Alias)
	}

	if key.Type() != keyfile.ECDSA {
		return nil, errors.Errorf("signer %q is not an ecdsa key", cfg.Alias)
	}

	return crypto.ToECDSA(key.PrivateKey())
}

// EVMAddressOf derives the EVM address of a configured local ECDSA signer.
func EVMAddressOf(cfg config.SignerConfig) (string, error) {
	key, err := LocalECDSAKey(cfg)
	if err != nil {
		return "", err
	}

	return crypto.PubkeyToAddress(key.PublicKey).Hex(), nil
}
