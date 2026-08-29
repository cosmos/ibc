// SPDX-License-Identifier: Apache-2.0

package config

import (
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
func (c Config) WithPatch(p Patch) (Config, []Conflict) {
	out := c
	var conflicts []Conflict

	out.Chains = mergeChains(c.Chains, p.Chains, &conflicts)
	out.Relayer.Connections = mergeConnections(c.Relayer.Connections, p.Connections, &conflicts)
	out.Attestors = mergeAttestors(c.Attestors, p.Attestors, &conflicts)

	return out, conflicts
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

func mergeConnections(existing, incoming []ConnectionConfig, conflicts *[]Conflict) []ConnectionConfig {
	out := append([]ConnectionConfig(nil), existing...)

	for _, conn := range incoming {
		idx := slices.IndexFunc(out, func(c ConnectionConfig) bool {
			return c.Alias == conn.Alias
		})
		if idx < 0 {
			out = append(out, conn)

			continue
		}

		if reflect.DeepEqual(out[idx], conn) {
			continue
		}

		*conflicts = append(*conflicts, Conflict{Kind: "connection", ID: conn.Alias})

		out[idx] = conn
	}

	return out
}

func mergeAttestors(existing, incoming Attestors, conflicts *[]Conflict) Attestors {
	out := append(Attestors(nil), existing...)

	for _, attestor := range incoming {
		id := attestorID(attestor)
		idx := slices.IndexFunc(out, func(a AttestorConfig) bool {
			return attestorID(a) == id
		})
		if idx < 0 {
			out = append(out, attestor)

			continue
		}

		if reflect.DeepEqual(out[idx], attestor) {
			continue
		}

		*conflicts = append(*conflicts, Conflict{Kind: "attestor", ID: id})

		out[idx] = attestor
	}

	return out
}

func attestorID(a AttestorConfig) string {
	id := string(a.Type) + " " + a.Name
	if a.GRPC != "" {
		id += " at " + a.GRPC
	}

	return id
}
