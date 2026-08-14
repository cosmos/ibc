// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/service/signer"
)

// resolveAttestorToken resolves one attestor token: an attestors[].name or a
// signers[] alias resolves through its key; anything else is passed through
// verbatim as an address, whose format only the target driver can judge.
func resolveAttestorToken(cfg config.Config, token string) (string, error) {
	alias := token
	if attestor, ok := cfg.AttestorByName(token); ok {
		if attestor.Type == config.AttestorTypeRemote {
			return "", errors.Errorf(
				"attestor %q is remote: its address isn't known statically, pass the address directly",
				token,
			)
		}
		alias = attestor.Signer
	}
	if sc, ok := cfg.Signer(alias); ok {
		return signer.EVMAddressOf(sc)
	}
	return token, nil
}

// attestorsForChain derives the default attestor set for clients tracking
// chainID: every configured attestor for that chain, resolved to an
// address. Errors when none are configured or any is unresolvable.
func attestorsForChain(cfg config.Config, chainID string) ([]string, error) {
	configured := cfg.AttestorsForChain(chainID)
	if len(configured) == 0 {
		return nil, errors.Errorf(
			"no attestors configured for chain %s: add attestors entries or pass --attestors <address|alias,...>",
			chainID,
		)
	}
	attestors := make([]string, 0, len(configured))
	for _, attestor := range configured {
		sc, ok := cfg.Signer(attestor.Signer)
		if !ok {
			return nil, errors.Errorf(
				"attestor %q references unknown signer %q",
				attestor.Name,
				attestor.Signer,
			)
		}
		address, err := signer.EVMAddressOf(sc)
		if err != nil {
			return nil, errors.Wrapf(err, "attestor %q", attestor.Name)
		}
		attestors = append(attestors, address)
	}
	return attestors, nil
}
