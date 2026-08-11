// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/service/signer"
)

// resolveAttestorToken resolves one attestor token: an
// attestor.attestations[].name or a signers[] alias resolves through its
// key; anything else is passed through verbatim as an address, whose format
// only the target driver can judge.
func resolveAttestorToken(cfg config.Config, token string) (string, error) {
	alias := token
	if attestation, ok := cfg.AttestationByName(token); ok {
		alias = attestation.Signer
	}
	if sc, ok := cfg.Signer(alias); ok {
		return signer.EVMAddressOf(sc)
	}
	return token, nil
}

// attestorsForChain derives the default attestor set for clients tracking
// chainID: every configured attestation for that chain, resolved to an
// address. Errors when none are configured or any is unresolvable.
func attestorsForChain(cfg config.Config, chainID string) ([]string, error) {
	attestations := cfg.AttestationsForChain(chainID)
	if len(attestations) == 0 {
		return nil, errors.Errorf(
			"no attestors configured for chain %s: add attestor.attestations entries or pass --attestors <address|alias,...>",
			chainID,
		)
	}
	attestors := make([]string, 0, len(attestations))
	for _, attestation := range attestations {
		sc, ok := cfg.Signer(attestation.Signer)
		if !ok {
			return nil, errors.Errorf(
				"attestation %q references unknown signer %q",
				attestation.Name,
				attestation.Signer,
			)
		}
		address, err := signer.EVMAddressOf(sc)
		if err != nil {
			return nil, errors.Wrapf(err, "attestation %q", attestation.Name)
		}
		attestors = append(attestors, address)
	}
	return attestors, nil
}
