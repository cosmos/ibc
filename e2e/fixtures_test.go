// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/harness/environment"
	"github.com/cosmos/ibc/harness/traffic"
)

// qbftMesh declares a fully connected graph of Besu QBFT Clients over chains:
// per Chain pair one Connection whose ends verify each other's sealed Besu
// headers, so it needs no Attestors. Every chain must run Besu QBFT.
func qbftMesh(chains []environment.ChainSpec) (environment.Spec, environment.Runtime) {
	spec := environment.Spec{Chains: slices.Clone(chains)}
	chainIDs := make([]environment.ChainID, 0, len(chains))
	for _, chain := range chains {
		chainIDs = append(chainIDs, chain.ChainID())
	}
	slices.Sort(chainIDs)

	for _, id := range chainIDs {
		spec.IBCInstances = append(spec.IBCInstances, environment.NewIBCInstance{
			ID: traffic.MeshInstanceID(id), Chain: id, Authority: traffic.ProtocolAuthorityID,
		})
	}
	for i, a := range chainIDs {
		for _, b := range chainIDs[i+1:] {
			spec.Connections = append(spec.Connections, environment.ConnectionSpec{
				ID: traffic.MeshConnectionID(a, b),
				A:  qbftMeshClient(a),
				B:  qbftMeshClient(b),
			})
		}
	}
	return spec, traffic.RuntimeWithProtocolDeployer(environment.Runtime{})
}

// qbftMeshClient trusts states for 14 days and tolerates a minute of clock drift.
func qbftMeshClient(chain environment.ChainID) environment.NewBesuQBFTClient {
	return environment.NewBesuQBFTClient{
		IBCInstance:    traffic.MeshInstanceID(chain),
		Authority:      traffic.ProtocolAuthorityID,
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
