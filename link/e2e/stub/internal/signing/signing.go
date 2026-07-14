// Package signing resolves the local ECDSA signers referenced by the stub config.
package signing

import (
	"crypto/ecdsa"
	"fmt"

	"github.com/ethereum/go-ethereum/crypto"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"
	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/service/signer/keyfile"
)

func LoadECDSA(signers []wire.Signer, alias string) (*ecdsa.PrivateKey, error) {
	var configured *wire.Signer
	for i := range signers {
		if signers[i].Alias != alias {
			continue
		}
		if configured != nil {
			return nil, fmt.Errorf("signer alias %q is configured more than once", alias)
		}
		configured = &signers[i]
	}
	if configured == nil {
		return nil, fmt.Errorf("signer alias %q is not configured", alias)
	}
	if configured.Type != wire.SignerTypeLocal {
		return nil, fmt.Errorf(
			"signer alias %q has type %q; the stub supports %q",
			alias,
			configured.Type,
			wire.SignerTypeLocal,
		)
	}
	if configured.File == "" {
		return nil, fmt.Errorf("local signer alias %q has no file", alias)
	}

	path, err := config.ExpandHome(configured.File)
	if err != nil {
		return nil, fmt.Errorf("expand local signer %q file: %w", alias, err)
	}
	keyType, privateKey, err := keyfile.Load(path)
	if err != nil {
		return nil, fmt.Errorf("load local signer %q from %q: %w", alias, path, err)
	}
	if keyType != keyfile.ECDSA {
		return nil, fmt.Errorf("local signer %q must contain an ECDSA key, got %q", alias, keyType)
	}
	parsed, err := crypto.ToECDSA(privateKey)
	if err != nil {
		return nil, fmt.Errorf("parse local signer %q ECDSA key: %w", alias, err)
	}
	return parsed, nil
}
