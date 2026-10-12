// SPDX-License-Identifier: Apache-2.0

package traffic

import (
	"fmt"
	"slices"

	"github.com/cosmos/ibc/e2e/internal/harness/environment"
)

// meshAttestorKeyBase offsets attestor keys past the protocol deployer key (0x05).
const meshAttestorKeyBase = 0x10

// AttestedMesh declares a fully connected attested graph over chains — per
// Chain pair one Connection of two attestation Clients, each backed by its own
// single-signature Attestor — and the Runtime binding every authority it
// needs, including the protocol deployer. The chains slice is copied.
func AttestedMesh(chains ...environment.ChainSpec) (environment.Spec, environment.Runtime) {
	authorities := map[environment.AuthorityID]environment.EVMAuthority{}
	nextKey := meshAttestorKeyBase
	spec := mesh(
		chains,
		func(chain environment.ChainID, connection environment.ConnectionID, end string) environment.ClientSpec {
			attestor := meshAttestorID(connection, end)
			authorities[environment.AuthorityID(attestor)] = environment.EVMAuthority{
				PrivateKeyHex: fmt.Sprintf("%064x", nextKey),
			}
			nextKey++
			return environment.NewClient{
				IBCInstance:           meshInstanceID(chain),
				Authority:             ProtocolAuthorityID,
				MinRequiredSignatures: 1,
				Attestors: []environment.AttestorSpec{{
					ID: attestor, Authority: environment.AuthorityID(attestor),
				}},
			}
		},
	)
	return spec, RuntimeWithProtocolDeployer(environment.Runtime{Authorities: authorities})
}

// QBFTMesh declares a fully connected graph of Besu QBFT Clients over chains:
// per Chain pair one Connection whose ends verify each other's sealed Besu
// headers, so it needs no Attestors. Every chain must run Besu QBFT. Clients
// trust states for 14 days and tolerate a minute of clock drift. The chains
// slice is copied.
func QBFTMesh(chains ...environment.ChainSpec) (environment.Spec, environment.Runtime) {
	spec := mesh(chains, func(chain environment.ChainID, _ environment.ConnectionID, _ string) environment.ClientSpec {
		return environment.NewBesuQBFTClient{
			IBCInstance:    meshInstanceID(chain),
			Authority:      ProtocolAuthorityID,
			TrustingPeriod: environment.BesuQBFTTrustingPeriod,
			MaxClockDrift:  60,
		}
	})
	return spec, RuntimeWithProtocolDeployer(environment.Runtime{})
}

// mesh declares one IBC Instance per chain and, per Chain pair in sorted
// order, one Connection whose ends client builds: end "a" on the first Chain
// of the pair and "b" on the second.
func mesh(
	chains []environment.ChainSpec,
	client func(chain environment.ChainID, connection environment.ConnectionID, end string) environment.ClientSpec,
) environment.Spec {
	spec := environment.Spec{Chains: slices.Clone(chains)}
	chainIDs := make([]environment.ChainID, 0, len(chains))
	for _, chain := range chains {
		chainIDs = append(chainIDs, chain.ChainID())
	}
	slices.Sort(chainIDs)

	for _, id := range chainIDs {
		spec.IBCInstances = append(spec.IBCInstances, environment.NewIBCInstance{
			ID: meshInstanceID(id), Chain: id, Authority: ProtocolAuthorityID,
		})
	}
	for i, a := range chainIDs {
		for _, b := range chainIDs[i+1:] {
			connectionID := meshConnectionID(a, b)
			spec.Connections = append(spec.Connections, environment.ConnectionSpec{
				ID: connectionID,
				A:  client(a, connectionID, "a"),
				B:  client(b, connectionID, "b"),
			})
		}
	}
	return spec
}

// MeshAttestorFor returns the Attestor backing the AttestedMesh Client hosted
// on chain and tracking counterparty, hiding that end labels follow sort order.
func MeshAttestorFor(chain, counterparty environment.ChainID) environment.AttestorID {
	end := "a"
	if counterparty < chain {
		end = "b"
	}
	return meshAttestorID(meshConnectionID(chain, counterparty), end)
}

// meshInstanceID names the IBC Instance a mesh declares on chain.
func meshInstanceID(chain environment.ChainID) environment.IBCInstanceID {
	return environment.IBCInstanceID(fmt.Sprintf("ibc-%s", chain))
}

// meshConnectionID names the Connection between a and b. It orders its
// arguments so callers need not know which Chain the mesh sorted first.
func meshConnectionID(a, b environment.ChainID) environment.ConnectionID {
	if b < a {
		a, b = b, a
	}
	return environment.ConnectionID(fmt.Sprintf("conn-%s-%s", a, b))
}

func meshAttestorID(connection environment.ConnectionID, end string) environment.AttestorID {
	return environment.AttestorID(fmt.Sprintf("%s-%s-attestor", connection, end))
}
