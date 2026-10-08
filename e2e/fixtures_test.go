// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/e2etest"
	"github.com/cosmos/ibc/harness/environment"
	"github.com/cosmos/ibc/harness/traffic"
)

// attestedMesh declares a fully connected attested graph over chains; see
// traffic.AttestedMesh, which the load-test binary shares.
func attestedMesh(chains []environment.ChainSpec) (environment.Spec, environment.Runtime) {
	return traffic.AttestedMesh(chains...)
}

// meshAttestorFor returns the Attestor backing the mesh Client hosted on chain
// and tracking counterparty, hiding that end labels follow sort order.
func meshAttestorFor(chain, counterparty environment.ChainID) environment.AttestorID {
	return traffic.MeshAttestorFor(chain, counterparty)
}

func fixtureChainID(declaration environment.ChainSpec) environment.ChainID {
	return declaration.ChainID()
}

func fixtureInstanceID(id environment.ChainID) environment.IBCInstanceID {
	return traffic.MeshInstanceID(id)
}

// fixtureConnectionID orders its arguments so callers need not know which
// Chain the mesh sorted first.
func fixtureConnectionID(a, b environment.ChainID) environment.ConnectionID {
	return traffic.MeshConnectionID(a, b)
}

func TestAttestedMesh(t *testing.T) {
	spec, runtime := attestedMesh([]environment.ChainSpec{
		environment.ManagedAnvil{ID: "chain-b", EVMChainID: 2},
		environment.ManagedAnvil{ID: "chain-a", EVMChainID: 1},
	})

	// The full declaration is pinned by traffic's tests; e2e relies on the
	// helpers agreeing with the IDs the mesh declares.
	require.NoError(t, environment.Validate(spec, runtime))
	require.Len(t, spec.Connections, 1)
	require.Equal(t, fixtureConnectionID("chain-b", "chain-a"), spec.Connections[0].ID)
	for _, chain := range []environment.ChainID{"chain-a", "chain-b"} {
		counterparty := environment.ChainID("chain-b")
		if chain == counterparty {
			counterparty = "chain-a"
		}
		_, ok := runtime.Authorities[environment.AuthorityID(meshAttestorFor(chain, counterparty))]
		require.True(t, ok, "attestor for %s must have an authority", chain)
	}
}

// qbftMesh declares a fully connected graph of Besu QBFT Clients over chains:
// per Chain pair one Connection whose ends verify each other's sealed Besu
// headers, so it needs no Attestors. Every chain must run Besu QBFT.
func qbftMesh(chains []environment.ChainSpec) (environment.Spec, environment.Runtime) {
	spec := environment.Spec{Chains: slices.Clone(chains)}
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
	for i, a := range chainIDs {
		for _, b := range chainIDs[i+1:] {
			spec.Connections = append(spec.Connections, environment.ConnectionSpec{
				ID: fixtureConnectionID(a, b),
				A:  qbftMeshClient(a),
				B:  qbftMeshClient(b),
			})
		}
	}
	return spec, e2etest.RuntimeWithProtocolDeployer(environment.Runtime{})
}

// qbftMeshClient trusts states for 14 days and tolerates a minute of clock drift.
func qbftMeshClient(chain environment.ChainID) environment.NewBesuQBFTClient {
	return environment.NewBesuQBFTClient{
		IBCInstance:    fixtureInstanceID(chain),
		Authority:      e2etest.ProtocolAuthorityID,
		TrustingPeriod: environment.BesuQBFTTrustingPeriod,
		MaxClockDrift:  60,
	}
}

func TestQBFTMesh(t *testing.T) {
	spec, runtime := qbftMesh([]environment.ChainSpec{
		environment.ManagedBesu{ID: "chain-b", EVMChainID: 2},
		environment.ManagedBesu{ID: "chain-a", EVMChainID: 1},
	})

	require.NoError(t, environment.Validate(spec, runtime))
	require.Len(t, spec.Chains, 2)
	require.Len(t, spec.IBCInstances, 2)
	require.Len(t, spec.Connections, 1)
	ends := []environment.ClientSpec{spec.Connections[0].A, spec.Connections[0].B}
	for i, chain := range []environment.ChainID{"chain-a", "chain-b"} {
		instance, ok := spec.IBCInstances[i].(environment.NewIBCInstance)
		require.True(t, ok)
		require.Equal(t, chain, instance.Chain)
		// QBFT endpoints carry no attestors and follow the sorted host instances.
		client, ok := ends[i].(environment.NewBesuQBFTClient)
		require.True(t, ok)
		require.Equal(t, instance.ID, client.IBCInstance)
	}
}
