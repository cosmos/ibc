// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
)

// attestedMesh declares a fully connected attested graph over chains — per
// Chain pair one Connection of two attestation Clients, each backed by its own
// single-signature Attestor — and the Runtime binding every authority it needs.
func attestedMesh(chains []environment.ChainSpec) (environment.Spec, environment.Runtime) {
	spec := environment.Spec{Chains: slices.Clone(chains)}
	authorities := map[environment.AuthorityID]environment.EVMAuthority{}
	chainIDs := make([]environment.ChainID, 0, len(chains))
	for _, chain := range chains {
		chainIDs = append(chainIDs, fixtureChainID(chain))
	}
	slices.Sort(chainIDs)

	for _, id := range chainIDs {
		spec.IBCInstances = append(spec.IBCInstances, environment.NewIBCInstance{
			ID: fixtureInstanceID(id), Chain: id, Authority: e2etest.ProtocolAuthorityID,
		})
	}
	nextKey := 0x10 // offset past the protocol deployer key (0x05)
	for i, a := range chainIDs {
		for _, b := range chainIDs[i+1:] {
			connectionID := fixtureConnectionID(a, b)
			spec.Connections = append(spec.Connections, environment.ConnectionSpec{
				ID: connectionID,
				A:  meshClient(connectionID, "a", a),
				B:  meshClient(connectionID, "b", b),
			})
			for _, end := range []string{"a", "b"} {
				attestor := meshAttestorID(connectionID, end)
				authority := environment.AuthorityID(attestor)
				spec.Attestors = append(spec.Attestors, environment.AttestorSpec{
					ID: attestor, Client: fixtureClientID(connectionID, end), Authority: authority,
				})
				authorities[authority] = environment.EVMAuthority{
					PrivateKeyHex: fmt.Sprintf("%064x", nextKey),
				}
				nextKey++
			}
		}
	}
	return spec, e2etest.RuntimeWithProtocolDeployer(environment.Runtime{Authorities: authorities})
}

func meshClient(connection environment.ConnectionID, end string, chain environment.ChainID) environment.NewClient {
	return environment.NewClient{
		ID:                    fixtureClientID(connection, end),
		IBCInstance:           fixtureInstanceID(chain),
		Authority:             e2etest.ProtocolAuthorityID,
		MinRequiredSignatures: 1,
	}
}

func meshAttestorID(connection environment.ConnectionID, end string) environment.AttestorID {
	return environment.AttestorID(fmt.Sprintf("%s-attestor", fixtureClientID(connection, end)))
}

// meshAttestorFor returns the Attestor backing the mesh Client hosted on chain
// and tracking counterparty, hiding that end labels follow sort order.
func meshAttestorFor(chain, counterparty environment.ChainID) environment.AttestorID {
	end := "a"
	if counterparty < chain {
		end = "b"
	}
	return meshAttestorID(fixtureConnectionID(chain, counterparty), end)
}

func fixtureChainID(declaration environment.ChainSpec) environment.ChainID {
	switch chain := declaration.(type) {
	case environment.ManagedAnvil:
		return chain.ID
	case environment.ManagedBesu:
		return chain.ID
	case environment.AttachedEVM:
		return chain.ID
	default:
		panic(fmt.Sprintf("e2e: unsupported Chain declaration %T", declaration))
	}
}

func fixtureInstanceID(id environment.ChainID) environment.IBCInstanceID {
	return environment.IBCInstanceID(fmt.Sprintf("ibc-%s", id))
}

// fixtureConnectionID orders its arguments so callers need not know which
// Chain the mesh sorted first.
func fixtureConnectionID(a, b environment.ChainID) environment.ConnectionID {
	if b < a {
		a, b = b, a
	}
	return environment.ConnectionID(fmt.Sprintf("conn-%s-%s", a, b))
}

func fixtureClientID(connection environment.ConnectionID, end string) environment.ClientID {
	return environment.ClientID(fmt.Sprintf("%s-%s", connection, end))
}

func TestAttestedMesh(t *testing.T) {
	chains := []environment.ChainSpec{
		environment.ManagedAnvil{ID: "chain-b", EVMChainID: 2},
		environment.ManagedAnvil{ID: "chain-a", EVMChainID: 1},
	}
	spec, runtime := attestedMesh(chains)
	chains[0] = environment.ManagedAnvil{ID: "changed", EVMChainID: 4}

	want := environment.Spec{
		Chains: []environment.ChainSpec{
			environment.ManagedAnvil{ID: "chain-b", EVMChainID: 2},
			environment.ManagedAnvil{ID: "chain-a", EVMChainID: 1},
		},
		IBCInstances: []environment.IBCInstanceSpec{
			environment.NewIBCInstance{ID: "ibc-chain-a", Chain: "chain-a", Authority: e2etest.ProtocolAuthorityID},
			environment.NewIBCInstance{ID: "ibc-chain-b", Chain: "chain-b", Authority: e2etest.ProtocolAuthorityID},
		},
		Connections: []environment.ConnectionSpec{{
			ID: "conn-chain-a-chain-b",
			A: environment.NewClient{
				ID:                    "conn-chain-a-chain-b-a",
				IBCInstance:           "ibc-chain-a",
				Authority:             e2etest.ProtocolAuthorityID,
				MinRequiredSignatures: 1,
			},
			B: environment.NewClient{
				ID:                    "conn-chain-a-chain-b-b",
				IBCInstance:           "ibc-chain-b",
				Authority:             e2etest.ProtocolAuthorityID,
				MinRequiredSignatures: 1,
			},
		}},
		Attestors: []environment.AttestorSpec{
			{
				ID:        "conn-chain-a-chain-b-a-attestor",
				Client:    "conn-chain-a-chain-b-a",
				Authority: "conn-chain-a-chain-b-a-attestor",
			},
			{
				ID:        "conn-chain-a-chain-b-b-attestor",
				Client:    "conn-chain-a-chain-b-b",
				Authority: "conn-chain-a-chain-b-b-attestor",
			},
		},
	}
	require.Equal(t, want, spec)

	// The hosted-on-chain lookup must be argument-order insensitive even
	// though the underlying end labels follow sort order.
	require.Equal(t, environment.AttestorID("conn-chain-a-chain-b-a-attestor"), meshAttestorFor("chain-a", "chain-b"))
	require.Equal(t, environment.AttestorID("conn-chain-a-chain-b-b-attestor"), meshAttestorFor("chain-b", "chain-a"))

	keys := map[string]struct{}{}
	for _, authority := range runtime.Authorities {
		keys[authority.PrivateKeyHex] = struct{}{}
	}
	require.Len(t, keys, len(spec.Attestors)+1, "attestor keys must be distinct from each other and the deployer")
	require.NoError(t, environment.Validate(spec, runtime))
}

func TestAttestedMeshConnectsEveryChainPair(t *testing.T) {
	spec, runtime := attestedMesh([]environment.ChainSpec{
		environment.ManagedAnvil{ID: "chain-c", EVMChainID: 3},
		environment.ManagedAnvil{ID: "chain-a", EVMChainID: 1},
		environment.ManagedAnvil{ID: "chain-b", EVMChainID: 2},
	})

	connectionIDs := make([]environment.ConnectionID, 0, len(spec.Connections))
	for _, connection := range spec.Connections {
		connectionIDs = append(connectionIDs, connection.ID)
	}
	require.Equal(t, []environment.ConnectionID{
		"conn-chain-a-chain-b", "conn-chain-a-chain-c", "conn-chain-b-chain-c",
	}, connectionIDs)
	require.NoError(t, environment.Validate(spec, runtime))
}
