// SPDX-License-Identifier: Apache-2.0

package e2etest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/besu"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
)

func TestAttestedMesh(t *testing.T) {
	chains := []environment.ChainSpec{
		environment.ManagedAnvil{ID: "chain-b", EVMChainID: 2},
		environment.ManagedAnvil{ID: "chain-a", EVMChainID: 1},
	}
	spec, runtime := AttestedMesh(chains)
	chains[0] = environment.ManagedAnvil{ID: "changed", EVMChainID: 4}

	want := environment.Spec{
		Chains: []environment.ChainSpec{
			environment.ManagedAnvil{ID: "chain-b", EVMChainID: 2},
			environment.ManagedAnvil{ID: "chain-a", EVMChainID: 1},
		},
		IBCInstances: []environment.IBCInstanceSpec{
			environment.NewIBCInstance{ID: "ibc-chain-a", Chain: "chain-a", Authority: ProtocolAuthorityID},
			environment.NewIBCInstance{ID: "ibc-chain-b", Chain: "chain-b", Authority: ProtocolAuthorityID},
		},
		Connections: []environment.ConnectionSpec{{
			ID: "conn-chain-a-chain-b",
			A: environment.NewClient{
				IBCInstance:           "ibc-chain-a",
				Authority:             ProtocolAuthorityID,
				MinRequiredSignatures: 1,
				Attestors: []environment.AttestorSpec{{
					ID: "conn-chain-a-chain-b-a-attestor", Authority: "conn-chain-a-chain-b-a-attestor",
				}},
			},
			B: environment.NewClient{
				IBCInstance:           "ibc-chain-b",
				Authority:             ProtocolAuthorityID,
				MinRequiredSignatures: 1,
				Attestors: []environment.AttestorSpec{{
					ID: "conn-chain-a-chain-b-b-attestor", Authority: "conn-chain-a-chain-b-b-attestor",
				}},
			},
		}},
	}
	require.Equal(t, want, spec)

	// The hosted-on-chain lookup must be argument-order insensitive even
	// though the underlying end labels follow sort order.
	require.Equal(t, environment.AttestorID("conn-chain-a-chain-b-a-attestor"), MeshAttestorFor("chain-a", "chain-b"))
	require.Equal(t, environment.AttestorID("conn-chain-a-chain-b-b-attestor"), MeshAttestorFor("chain-b", "chain-a"))

	keys := map[string]struct{}{}
	for _, authority := range runtime.Authorities {
		keys[authority.PrivateKeyHex] = struct{}{}
	}
	require.Len(t, keys, len(runtime.Authorities), "attestor keys must be distinct from each other and the deployer")
	require.NoError(t, environment.Validate(spec, runtime))
}

func TestAttestedMeshConnectsEveryChainPair(t *testing.T) {
	spec, runtime := AttestedMesh([]environment.ChainSpec{
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

func TestBesuMesh(t *testing.T) {
	spec, runtime := BesuMesh([]environment.ChainSpec{
		environment.ManagedBesu{ID: "chain-b", EVMChainID: 2, Consensus: besu.IBFT2},
		environment.ManagedBesu{ID: "chain-a", EVMChainID: 1, Consensus: besu.IBFT2},
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
		// Besu endpoints carry no attestors and follow the sorted host instances.
		client, ok := ends[i].(environment.NewBesuClient)
		require.True(t, ok)
		require.Equal(t, instance.ID, client.IBCInstance)
	}
}
