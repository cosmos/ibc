// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"reflect"
	"slices"
)

// DeploymentConfig contains settings projected from deployment manifests.
// Connection aliases are suggestions; reconciliation allocates unused names for new pairs.
type DeploymentConfig struct {
	Chains      []ChainConfig
	Connections []ConnectionConfig
	Attestors   Attestors
}

// Conflict names an existing config entry deployment reconciliation would overwrite.
type Conflict struct {
	Kind string
	ID   string
}

func (c Conflict) String() string {
	return c.Kind + " " + c.ID
}

// ReconcileDeployment merges generated deployment settings into c, preserving
// existing operational choices. Entries are matched by identity, not by alias.
// Both c and the result must have unambiguous identities.
func (c Config) ReconcileDeployment(d DeploymentConfig) (Config, []Conflict, error) {
	if err := c.ValidateIdentities(); err != nil {
		return Config{}, nil, err
	}

	out := c
	var conflicts []Conflict
	var err error

	out.Chains = mergeChains(c.Chains, d.Chains, &conflicts)
	out.Relayer.Connections, err = mergeConnections(c.Relayer.Connections, d.Connections, &conflicts)
	if err != nil {
		return Config{}, nil, err
	}
	out.Attestors, err = mergeAttestors(c.Attestors, d.Attestors)
	if err != nil {
		return Config{}, nil, err
	}
	if err := out.ValidateIdentities(); err != nil {
		return Config{}, nil, err
	}

	return out, conflicts, nil
}

// mergeChains replaces a chain wholesale. Rendering starts from the existing
// chain entry and only sets the deployed router, so a conflict here means the
// router actually changed.
func mergeChains(existing, incoming []ChainConfig, conflicts *[]Conflict) []ChainConfig {
	out := append([]ChainConfig(nil), existing...)

	for _, chain := range incoming {
		idx := slices.IndexFunc(out, func(c ChainConfig) bool {
			return c.ChainID == chain.ChainID
		})
		if idx < 0 {
			out = append(out, chain)

			continue
		}

		if reflect.DeepEqual(out[idx], chain) {
			continue
		}

		*conflicts = append(*conflicts, Conflict{Kind: "chain", ID: chain.ChainID})

		out[idx] = chain
	}

	return out
}

func sameConnection(a, b ConnectionConfig) bool {
	return (a.ClientA.identity() == b.ClientA.identity() && a.ClientB.identity() == b.ClientB.identity()) ||
		(a.ClientA.identity() == b.ClientB.identity() && a.ClientB.identity() == b.ClientA.identity())
}

// mergeConnections matches connections by their client pair regardless of alias
// or A/B order. On a match only an explicit (nonempty) generated signer is
// applied; everything else stays as configured.
func mergeConnections(existing, incoming []ConnectionConfig, conflicts *[]Conflict) ([]ConnectionConfig, error) {
	out := append([]ConnectionConfig(nil), existing...)
	for _, conn := range incoming {
		idx := slices.IndexFunc(out, func(c ConnectionConfig) bool { return sameConnection(c, conn) })
		if idx < 0 {
			base := conn.Alias
			for suffix := 1; slices.ContainsFunc(out, func(c ConnectionConfig) bool { return c.Alias == conn.Alias }); suffix++ {
				conn.Alias = fmt.Sprintf("%s-%d", base, suffix)
			}
			out = append(out, conn)
			continue
		}
		merged := out[idx]
		changed := false
		for _, incomingEnd := range []ClientEnd{conn.ClientA, conn.ClientB} {
			end := &merged.ClientA
			if end.identity() != incomingEnd.identity() {
				end = &merged.ClientB
			}
			// A remote prover is an operational choice, not an on-chain client type.
			if end.Type != ClientTypeRemote && end.Type != incomingEnd.Type {
				return nil, fmt.Errorf("connection %q: client %q on chain %q has type %q, manifest has %q; automatic client-type changes are not supported, even when the router changes: explicitly update the client's type and compatible params in the config before rerunning",
					merged.Alias, end.ClientID, end.ChainID, end.Type, incomingEnd.Type)
			}
			if incomingEnd.Signer != "" && incomingEnd.Signer != end.Signer {
				changed = true
				end.Signer = incomingEnd.Signer
			}
		}
		if changed {
			*conflicts = append(*conflicts, Conflict{Kind: "connection", ID: merged.Alias})
			out[idx] = merged
		}
	}
	return out, nil
}

// mergeAttestors never overwrites an existing attestor. Remote attestors are
// matched by name and host. Local attestors are matched by name first, so a
// draft can have its signer filled in place, and then by chain/signer, so an
// operator's renamed attestor is kept rather than duplicated.
func mergeAttestors(existing, incoming Attestors) (Attestors, error) {
	out := append(Attestors(nil), existing...)
	for _, a := range incoming {
		if a.Type == AttestorTypeRemote {
			if !slices.ContainsFunc(out, func(b AttestorConfig) bool {
				return b.Type == AttestorTypeRemote && b.Name == a.Name && b.GRPC == a.GRPC
			}) {
				out = append(out, a)
			}
			continue
		}

		idx := slices.IndexFunc(out, func(b AttestorConfig) bool {
			return b.Type == AttestorTypeLocal && b.Name == a.Name
		})
		if idx >= 0 {
			b := &out[idx]
			if b.ChainID != a.ChainID || (b.Signer != "" && a.Signer != "" && b.Signer != a.Signer) {
				return nil, fmt.Errorf("local attestor name %q already names a different chain/signer", a.Name)
			}
			if b.Signer == "" {
				b.Signer = a.Signer
			}
			continue
		}

		if a.Signer != "" && slices.ContainsFunc(out, func(b AttestorConfig) bool {
			return b.Type == AttestorTypeLocal && b.ChainID == a.ChainID && b.Signer == a.Signer
		}) {
			continue
		}
		out = append(out, a)
	}
	return out, nil
}
