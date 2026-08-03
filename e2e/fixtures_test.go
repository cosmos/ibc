package e2e_test

import (
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/cosmos/ibc/e2e/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
)

func dummyClientMeshSpec(chains []environment.ChainSpec) environment.Spec {
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
			connectionID := fixtureConnectionID(a, b)
			spec.Connections = append(spec.Connections, environment.ConnectionSpec{
				ID: connectionID,
				A: environment.DummyClient{
					ID:          fixtureClientID(connectionID, "a"),
					IBCInstance: fixtureInstanceID(a),
					Authority:   e2etest.ProtocolAuthorityID,
				},
				B: environment.DummyClient{
					ID:          fixtureClientID(connectionID, "b"),
					IBCInstance: fixtureInstanceID(b),
					Authority:   e2etest.ProtocolAuthorityID,
				},
			})
		}
	}
	return spec
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

func fixtureConnectionID(a, b environment.ChainID) environment.ConnectionID {
	return environment.ConnectionID(fmt.Sprintf("conn-%s-%s", a, b))
}

func fixtureClientID(connection environment.ConnectionID, end string) environment.ClientID {
	return environment.ClientID(fmt.Sprintf("%s-%s", connection, end))
}

func TestDummyClientMeshSpec(t *testing.T) {
	chains := []environment.ChainSpec{
		environment.ManagedAnvil{ID: "chain-b", EVMChainID: 2},
		environment.ManagedAnvil{ID: "chain-a", EVMChainID: 1},
		environment.ManagedBesu{ID: "chain-c", EVMChainID: 3},
	}
	got := dummyClientMeshSpec(chains)
	chains[0] = environment.ManagedAnvil{ID: "changed", EVMChainID: 4}

	want := environment.Spec{
		Chains: []environment.ChainSpec{
			environment.ManagedAnvil{ID: "chain-b", EVMChainID: 2},
			environment.ManagedAnvil{ID: "chain-a", EVMChainID: 1},
			environment.ManagedBesu{ID: "chain-c", EVMChainID: 3},
		},
		IBCInstances: []environment.IBCInstanceSpec{
			environment.NewIBCInstance{ID: "ibc-chain-a", Chain: "chain-a", Authority: e2etest.ProtocolAuthorityID},
			environment.NewIBCInstance{ID: "ibc-chain-b", Chain: "chain-b", Authority: e2etest.ProtocolAuthorityID},
			environment.NewIBCInstance{ID: "ibc-chain-c", Chain: "chain-c", Authority: e2etest.ProtocolAuthorityID},
		},
		Connections: []environment.ConnectionSpec{
			dummyClientConnection("chain-a", "chain-b"),
			dummyClientConnection("chain-a", "chain-c"),
			dummyClientConnection("chain-b", "chain-c"),
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dummyClientMeshSpec() = %#v, want %#v", got, want)
	}
}

func dummyClientConnection(a, b environment.ChainID) environment.ConnectionSpec {
	id := fixtureConnectionID(a, b)
	return environment.ConnectionSpec{
		ID: id,
		A: environment.DummyClient{
			ID:          fixtureClientID(id, "a"),
			IBCInstance: fixtureInstanceID(a),
			Authority:   e2etest.ProtocolAuthorityID,
		},
		B: environment.DummyClient{
			ID:          fixtureClientID(id, "b"),
			IBCInstance: fixtureInstanceID(b),
			Authority:   e2etest.ProtocolAuthorityID,
		},
	}
}
