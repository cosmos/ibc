// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"reflect"
	"slices"
)

// Patch is the set of config sections projected out of deployment manifests.
type Patch struct {
	Chains      []ChainConfig
	Connections []ConnectionConfig
	Attestors   Attestors
}

// Conflict names an existing config entry a Patch would overwrite.
type Conflict struct {
	Kind string
	ID   string
}

func (c Conflict) String() string {
	return c.Kind + " " + c.ID
}

// WithPatch returns c with p merged in, alongside the entries it overwrites.
func (c Config) WithPatch(p Patch) (Config, []Conflict, error) {
	if err := c.ValidateIdentities(); err != nil {
		return Config{}, nil, err
	}
	out := c
	var conflicts []Conflict

	out.Chains = mergeChains(c.Chains, p.Chains, &conflicts)
	var err error
	out.Relayer.Connections, err = mergeConnections(c.Relayer.Connections, p.Connections, &conflicts)
	if err != nil {
		return Config{}, nil, err
	}
	out.Attestors, err = mergeAttestors(c.Attestors, p.Attestors)
	if err != nil {
		return Config{}, nil, err
	}

	return out, conflicts, out.ValidateIdentities()
}

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

// ClientEndIdentity is the identity of a light client, independent of aliases.
type ClientEndIdentity struct{ ChainID, ClientID string }

func (c ClientEnd) Identity() ClientEndIdentity {
	return ClientEndIdentity{c.ChainID, c.ClientID}
}

func sameConnection(a, b ConnectionConfig) bool {
	return (a.ClientA.Identity() == b.ClientA.Identity() && a.ClientB.Identity() == b.ClientB.Identity()) ||
		(a.ClientA.Identity() == b.ClientB.Identity() && a.ClientB.Identity() == b.ClientA.Identity())
}

func mergeConnections(existing, incoming []ConnectionConfig, conflicts *[]Conflict) ([]ConnectionConfig, error) {
	out := append([]ConnectionConfig(nil), existing...)
	for _, conn := range incoming {
		idx := slices.IndexFunc(out, func(c ConnectionConfig) bool { return sameConnection(c, conn) })
		if idx < 0 {
			if slices.ContainsFunc(out, func(c ConnectionConfig) bool { return c.Alias == conn.Alias }) {
				return nil, fmt.Errorf("connection alias %q already names a different client pair", conn.Alias)
			}
			out = append(out, conn)
			continue
		}
		merged := out[idx]
		for _, incomingEnd := range []ClientEnd{conn.ClientA, conn.ClientB} {
			end := &merged.ClientA
			if end.Identity() != incomingEnd.Identity() {
				end = &merged.ClientB
			}
			// A remote prover is an operational choice, not an on-chain client type.
			if end.Type != ClientTypeRemote && end.Type != incomingEnd.Type {
				return nil, fmt.Errorf("connection %q: client %q on chain %q has type %q, manifest has %q",
					merged.Alias, end.ClientID, end.ChainID, end.Type, incomingEnd.Type)
			}
			// A nonempty signer in a patch is an explicit override. Other operational
			// settings belong to the existing configuration, not generated defaults.
			if incomingEnd.Signer != "" {
				end.Signer = incomingEnd.Signer
			}
		}
		if !reflect.DeepEqual(out[idx], merged) {
			*conflicts = append(*conflicts, Conflict{Kind: "connection", ID: merged.Alias})
			out[idx] = merged
		}
	}
	return out, nil
}

func sameAttestor(a, b AttestorConfig) bool {
	if a.Type != b.Type {
		return false
	}
	if a.Type == AttestorTypeRemote {
		return a.Name == b.Name && a.GRPC == b.GRPC
	}
	if a.Signer == "" || b.Signer == "" {
		return a.Signer == b.Signer && a.Name == b.Name && a.ChainID == b.ChainID
	}
	return a.ChainID == b.ChainID && a.Signer == b.Signer
}

func mergeAttestors(existing, incoming Attestors) (Attestors, error) {
	out := append(Attestors(nil), existing...)
	for _, a := range incoming {
		if slices.ContainsFunc(out, func(b AttestorConfig) bool { return sameAttestor(a, b) }) {
			continue
		}
		if a.Type == AttestorTypeLocal && slices.ContainsFunc(out, func(b AttestorConfig) bool {
			return b.Type == AttestorTypeLocal && a.Name == b.Name
		}) {
			return nil, fmt.Errorf("local attestor name %q already names a different chain/signer", a.Name)
		}
		out = append(out, a)
	}
	return out, nil
}

// ValidateIdentities checks structural uniqueness without requiring generated
// TODO fields to be runnable. Full validation uses these same checks.
func (c Config) ValidateIdentities() error {
	if err := c.Chains.validateIdentities(); err != nil {
		return errPath("chains", err)
	}
	if err := c.Relayer.validateConnectionIdentities(); err != nil {
		return errPath("relayer", err)
	}
	if err := c.Attestors.validateIdentities(); err != nil {
		return errPath("attestors", err)
	}
	return nil
}
