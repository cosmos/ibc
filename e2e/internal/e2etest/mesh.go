// SPDX-License-Identifier: Apache-2.0

package e2etest

import (
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/besu"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
)

// AttestedMesh declares a fully connected attested graph over chains — per
// Chain pair one Connection of two attestation Clients, each backed by its own
// single-signature Attestor — and the Runtime binding every authority it needs.
func AttestedMesh(chains []environment.ChainSpec) (environment.Spec, environment.Runtime) {
	spec := environment.Spec{Chains: slices.Clone(chains)}
	authorities := map[environment.AuthorityID]environment.EVMAuthority{}
	chainIDs := make([]environment.ChainID, 0, len(chains))
	for _, chain := range chains {
		chainIDs = append(chainIDs, chainSpecID(chain))
	}
	slices.Sort(chainIDs)

	for _, id := range chainIDs {
		spec.IBCInstances = append(spec.IBCInstances, environment.NewIBCInstance{
			ID: meshInstanceID(id), Chain: id, Authority: ProtocolAuthorityID,
		})
	}
	nextKey := 0x10 // offset past the protocol deployer key (0x05)
	for i, a := range chainIDs {
		for _, b := range chainIDs[i+1:] {
			connectionID := MeshConnectionID(a, b)
			attestorA := meshAttestorID(connectionID, "a")
			attestorB := meshAttestorID(connectionID, "b")
			connection := environment.ConnectionSpec{
				ID: connectionID,
				A:  attestedMeshClient(a, attestorA),
				B:  attestedMeshClient(b, attestorB),
			}
			spec.Connections = append(spec.Connections, connection)
			for _, attestor := range []environment.AttestorID{attestorA, attestorB} {
				authority := environment.AuthorityID(attestor)
				authorities[authority] = environment.EVMAuthority{
					PrivateKeyHex: fmt.Sprintf("%064x", nextKey),
				}
				nextKey++
			}
		}
	}
	return spec, RuntimeWithProtocolDeployer(environment.Runtime{Authorities: authorities})
}

func attestedMeshClient(chain environment.ChainID, attestor environment.AttestorID) environment.NewClient {
	return environment.NewClient{
		IBCInstance:           meshInstanceID(chain),
		Authority:             ProtocolAuthorityID,
		MinRequiredSignatures: 1,
		Attestors: []environment.AttestorSpec{{
			ID: attestor, Authority: environment.AuthorityID(attestor),
		}},
	}
}

func meshAttestorID(connection environment.ConnectionID, end string) environment.AttestorID {
	return environment.AttestorID(fmt.Sprintf("%s-%s-attestor", connection, end))
}

// MeshAttestorFor returns the Attestor backing the mesh Client hosted on chain
// and tracking counterparty, hiding that end labels follow sort order.
func MeshAttestorFor(chain, counterparty environment.ChainID) environment.AttestorID {
	end := "a"
	if counterparty < chain {
		end = "b"
	}
	return meshAttestorID(MeshConnectionID(chain, counterparty), end)
}

func chainSpecID(declaration environment.ChainSpec) environment.ChainID {
	switch chain := declaration.(type) {
	case environment.ManagedAnvil:
		return chain.ID
	case environment.ManagedBesu:
		return chain.ID
	case environment.AttachedEVM:
		return chain.ID
	default:
		panic(fmt.Sprintf("e2etest: unsupported Chain declaration %T", declaration))
	}
}

func meshInstanceID(id environment.ChainID) environment.IBCInstanceID {
	return environment.IBCInstanceID(fmt.Sprintf("ibc-%s", id))
}

// MeshConnectionID orders its arguments so callers need not know which
// Chain the mesh sorted first.
func MeshConnectionID(a, b environment.ChainID) environment.ConnectionID {
	if b < a {
		a, b = b, a
	}
	return environment.ConnectionID(fmt.Sprintf("conn-%s-%s", a, b))
}

// BesuChains requests Besu Chains A and B running consensusA and consensusB.
func BesuChains(t testing.TB, consensusA, consensusB besu.Consensus) []environment.ChainSpec {
	t.Helper()
	requirements := EVMRequirements{Provider: EVMProviderBesu}
	chains := EVMChains(t, requirements, ChainA, ChainB)
	for i, consensus := range []besu.Consensus{consensusA, consensusB} {
		chain, ok := chains[i].(environment.ManagedBesu)
		require.True(t, ok)
		chain.Consensus = consensus
		chains[i] = chain
	}
	return chains
}

// BesuMesh declares a fully connected graph of Besu Clients over chains: per
// Chain pair one Connection whose ends verify each other's sealed Besu
// headers, so it needs no Attestors. Every chain must run Besu.
func BesuMesh(chains []environment.ChainSpec) (environment.Spec, environment.Runtime) {
	spec := environment.Spec{Chains: slices.Clone(chains)}
	chainIDs := make([]environment.ChainID, 0, len(chains))
	for _, chain := range chains {
		chainIDs = append(chainIDs, chainSpecID(chain))
	}
	slices.Sort(chainIDs)

	for _, id := range chainIDs {
		spec.IBCInstances = append(spec.IBCInstances, environment.NewIBCInstance{
			ID: meshInstanceID(id), Chain: id, Authority: ProtocolAuthorityID,
		})
	}
	for i, a := range chainIDs {
		for _, b := range chainIDs[i+1:] {
			spec.Connections = append(spec.Connections, environment.ConnectionSpec{
				ID: MeshConnectionID(a, b),
				A:  besuMeshClient(a),
				B:  besuMeshClient(b),
			})
		}
	}
	return spec, RuntimeWithProtocolDeployer(environment.Runtime{})
}

// besuMeshClient trusts states for 14 days and tolerates a minute of clock drift.
func besuMeshClient(chain environment.ChainID) environment.NewBesuClient {
	return environment.NewBesuClient{
		IBCInstance:    meshInstanceID(chain),
		Authority:      ProtocolAuthorityID,
		TrustingPeriod: environment.BesuTrustingPeriod,
		MaxClockDrift:  60,
	}
}
