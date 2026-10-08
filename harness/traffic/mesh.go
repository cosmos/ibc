// SPDX-License-Identifier: Apache-2.0

package traffic

import (
	"fmt"
	"slices"

	"github.com/cosmos/ibc/harness/environment"
)

// meshAttestorKeyBase offsets attestor keys past the protocol deployer key (0x05).
const meshAttestorKeyBase = 0x10

// AttestedMesh declares a fully connected attested graph over chains — per
// Chain pair one Connection of two attestation Clients, each backed by its own
// single-signature Attestor — and the Runtime binding every authority it
// needs, including the protocol deployer. The chains slice is copied.
func AttestedMesh(chains ...environment.ChainSpec) (environment.Spec, environment.Runtime) {
	spec := environment.Spec{Chains: slices.Clone(chains)}
	authorities := map[environment.AuthorityID]environment.EVMAuthority{}
	chainIDs := make([]environment.ChainID, 0, len(chains))
	for _, chain := range chains {
		chainIDs = append(chainIDs, chain.ChainID())
	}
	slices.Sort(chainIDs)

	for _, id := range chainIDs {
		spec.IBCInstances = append(spec.IBCInstances, environment.NewIBCInstance{
			ID: MeshInstanceID(id), Chain: id, Authority: ProtocolAuthorityID,
		})
	}
	nextKey := meshAttestorKeyBase
	for i, a := range chainIDs {
		for _, b := range chainIDs[i+1:] {
			connectionID := MeshConnectionID(a, b)
			attestorA := meshAttestorID(connectionID, "a")
			attestorB := meshAttestorID(connectionID, "b")
			spec.Connections = append(spec.Connections, environment.ConnectionSpec{
				ID: connectionID,
				A:  meshClient(a, attestorA),
				B:  meshClient(b, attestorB),
			})
			for _, attestor := range []environment.AttestorID{attestorA, attestorB} {
				authorities[environment.AuthorityID(attestor)] = environment.EVMAuthority{
					PrivateKeyHex: fmt.Sprintf("%064x", nextKey),
				}
				nextKey++
			}
		}
	}
	return spec, RuntimeWithProtocolDeployer(environment.Runtime{Authorities: authorities})
}

// MeshAttestorFor returns the Attestor backing the AttestedMesh Client hosted
// on chain and tracking counterparty, hiding that end labels follow sort order.
func MeshAttestorFor(chain, counterparty environment.ChainID) environment.AttestorID {
	end := "a"
	if counterparty < chain {
		end = "b"
	}
	return meshAttestorID(MeshConnectionID(chain, counterparty), end)
}

// MeshInstanceID names the IBC Instance a mesh declares on chain.
func MeshInstanceID(chain environment.ChainID) environment.IBCInstanceID {
	return environment.IBCInstanceID(fmt.Sprintf("ibc-%s", chain))
}

// MeshConnectionID names the Connection between a and b. It orders its
// arguments so callers need not know which Chain the mesh sorted first.
func MeshConnectionID(a, b environment.ChainID) environment.ConnectionID {
	if b < a {
		a, b = b, a
	}
	return environment.ConnectionID(fmt.Sprintf("conn-%s-%s", a, b))
}

func meshClient(chain environment.ChainID, attestor environment.AttestorID) environment.NewClient {
	return environment.NewClient{
		IBCInstance:           MeshInstanceID(chain),
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
