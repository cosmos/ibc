// SPDX-License-Identifier: Apache-2.0

package environment

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSpecValidateMixedGraph(t *testing.T) {
	spec := validSpec()
	require.NoError(t, spec.validate())

	// Multiple independently identified Attestors may serve the same Client.
	spec.Attestors = append(spec.Attestors, AttestorSpec{
		ID: "attestor-3", Client: spec.Connections[0].ARef(), Authority: "attest-a-2",
	})
	require.NoError(t, spec.validate())
}

func TestSpecValidateStrictVariants(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Spec)
		want   string
	}{
		{
			name: "attached endpoint binding is required",
			mutate: func(s *Spec) {
				chain := s.Chains[1].(AttachedEVM)
				chain.Endpoint = ""
				s.Chains[1] = chain
			},
			want: "endpoint binding is required",
		},
		{
			name: "attached timing is explicit",
			mutate: func(s *Spec) {
				chain := s.Chains[1].(AttachedEVM)
				chain.Timing = Timing{}
				s.Chains[1] = chain
			},
			want: "completion budget must be greater than zero",
		},
		{
			name: "new IBC Instance authority is required",
			mutate: func(s *Spec) {
				instance := s.IBCInstances[0].(NewIBCInstance)
				instance.Authority = ""
				s.IBCInstances[0] = instance
			},
			want: "IBC Instance \"ibc-a\" authority is required",
		},
		{
			name: "existing IBC Instance locator is required",
			mutate: func(s *Spec) {
				instance := s.IBCInstances[1].(ExistingIBCInstance)
				instance.Locator = ""
				s.IBCInstances[1] = instance
			},
			want: "IBC Instance \"ibc-b\" locator is required",
		},
		{
			name: "new IBC Client authority is required",
			mutate: func(s *Spec) {
				connection := s.Connections[0]
				client := connection.A.(NewClient)
				client.Authority = ""
				connection.A = client
				s.Connections[0] = connection
			},
			want: `IBC Client "connection-ab/A" authority is required`,
		},
		{
			name: "new IBC Client quorum is required",
			mutate: func(s *Spec) {
				connection := s.Connections[0]
				client := connection.A.(NewClient)
				client.MinRequiredSignatures = 0
				connection.A = client
				s.Connections[0] = connection
			},
			want: "minimum required signatures must be greater than zero",
		},
		{
			name: "existing IBC Client locator is required",
			mutate: func(s *Spec) {
				connection := existingConnectionSpec()
				client := connection.B.(ExistingClient)
				client.Locator = ""
				connection.B = client
				s.Connections[0] = connection
			},
			want: `IBC Client "connection-ab/B" locator is required`,
		},
		{
			name: "Attestor authority is required",
			mutate: func(s *Spec) {
				s.Attestors[0].Authority = ""
			},
			want: "Attestor \"attestor-1\" authority is required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spec := validSpec()
			tc.mutate(&spec)
			require.ErrorContains(t, spec.validate(), tc.want)
		})
	}
}

func TestSpecValidateNewClientRequiresAttestor(t *testing.T) {
	spec := validSpec()
	spec.Attestors = spec.Attestors[:1]
	require.ErrorContains(t, spec.validate(), `IBC Client "connection-ab/B" must have at least one Attestor`)
}

func TestSpecValidateNewClientQuorumDoesNotExceedAttestors(t *testing.T) {
	spec := validSpec()
	connection := spec.Connections[0]
	client := connection.A.(NewClient)
	client.MinRequiredSignatures = 2
	connection.A = client
	spec.Connections[0] = connection
	require.ErrorContains(t, spec.validate(), `IBC Client "connection-ab/A" requires 2 signatures from 1 Attestors`)
}

func TestSpecValidateConnectionEndCombinations(t *testing.T) {
	t.Run("both existing with Attestor reference", func(t *testing.T) {
		spec := validSpec()
		makeChainAAttached(&spec)
		spec.IBCInstances[0] = ExistingIBCInstance{ID: "ibc-a", Chain: "chain-a", Locator: "0xibc-a"}
		spec.Connections[0] = existingConnectionSpec()
		require.NoError(t, spec.validate())
	})

	t.Run("new and existing", func(t *testing.T) {
		spec := validSpec()
		makeChainAAttached(&spec)
		spec.IBCInstances[0] = ExistingIBCInstance{ID: "ibc-a", Chain: "chain-a", Locator: "0xibc-a"}
		connection := spec.Connections[0]
		connection.A = existingConnectionSpec().A
		spec.Connections[0] = connection
		require.NoError(t, spec.validate())
	})

	t.Run("existing Instance requires attached Chain", func(t *testing.T) {
		spec := validSpec()
		spec.IBCInstances[0] = ExistingIBCInstance{
			ID: "ibc-a", Chain: "chain-a", Locator: "0xibc-a",
		}
		require.ErrorContains(
			t,
			spec.validate(),
			`existing IBC Instance "ibc-a" must belong to an attached Chain, but "chain-a" is managed`,
		)
	})

	t.Run("existing Client cannot belong to new Instance", func(t *testing.T) {
		spec := validSpec()
		connection := spec.Connections[0]
		connection.A = existingConnectionSpec().A
		spec.Connections[0] = connection
		require.ErrorContains(
			t,
			spec.validate(),
			`existing IBC Client "connection-ab/A" cannot belong to new IBC Instance "ibc-a"`,
		)
	})
}

func TestSpecValidateIdentityAndReferences(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Spec)
		want   string
	}{
		{
			name: "duplicate Chain id",
			mutate: func(s *Spec) {
				s.Chains = append(s.Chains, ManagedAnvil{ID: "chain-a", EVMChainID: 31339})
			},
			want: "duplicate Chain id \"chain-a\"",
		},
		{
			name: "duplicate EVM chain id",
			mutate: func(s *Spec) {
				s.Chains = append(s.Chains, ManagedAnvil{ID: "chain-c", EVMChainID: 31337})
			},
			want: `Chains "chain-a" and "chain-c" use duplicate EVM chain id 31337`,
		},
		{
			name: "duplicate IBC Instance id",
			mutate: func(s *Spec) {
				s.IBCInstances = append(s.IBCInstances, NewIBCInstance{
					ID: "ibc-a", Chain: "chain-a", Authority: "deploy-a",
				})
			},
			want: "duplicate IBC Instance id \"ibc-a\"",
		},
		{
			name: "IBC Instance references unknown Chain",
			mutate: func(s *Spec) {
				instance := s.IBCInstances[0].(NewIBCInstance)
				instance.Chain = "missing"
				s.IBCInstances[0] = instance
			},
			want: "references unknown Chain \"missing\"",
		},
		{
			name: "duplicate IBC Connection id",
			mutate: func(s *Spec) {
				s.Connections = append(s.Connections, existingConnectionSpec())
			},
			want: "duplicate IBC Connection id \"connection-ab\"",
		},
		{
			name: "IBC Client references unknown IBC Instance",
			mutate: func(s *Spec) {
				connection := s.Connections[0]
				client := connection.B.(NewClient)
				client.IBCInstance = "missing"
				connection.B = client
				s.Connections[0] = connection
			},
			want: "references unknown IBC Instance \"missing\"",
		},
		{
			name: "Attestor references known IBC Client",
			mutate: func(s *Spec) {
				s.Attestors[0].Client.Connection = "missing"
			},
			want: `Attestor "attestor-1" references unknown IBC Client "missing/A"`,
		},
		{
			name: "Attestor reference end is valid",
			mutate: func(s *Spec) {
				s.Attestors[0].Client.End = 99
			},
			want: `has invalid Connection end ConnectionEnd(99)`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spec := validSpec()
			tc.mutate(&spec)
			require.ErrorContains(t, spec.validate(), tc.want)
		})
	}
}

func TestSpecValidateConnectionPair(t *testing.T) {
	t.Run("IBC Instances differ", func(t *testing.T) {
		spec := validSpec()
		connection := spec.Connections[0]
		clientA := connection.A.(NewClient)
		clientB := connection.B.(NewClient)
		clientB.IBCInstance = clientA.IBCInstance
		connection.B = clientB
		spec.Connections[0] = connection
		require.ErrorContains(t, spec.validate(), "clients must belong to distinct IBC Instances")
	})
}

func TestConnectionClientRefsAreStableAndDistinct(t *testing.T) {
	connection := ConnectionSpec{ID: "connection-ab"}
	copied := connection

	require.Equal(t, IBCClientRef{Connection: "connection-ab", End: ConnectionEndA}, connection.ARef())
	require.Equal(t, IBCClientRef{Connection: "connection-ab", End: ConnectionEndB}, connection.BRef())
	require.Equal(t, connection.ARef(), copied.ARef())
	require.NotEqual(t, connection.ARef(), connection.BRef())
}

func TestSpecValidateClientLocatorUniquenessIsInstanceScoped(t *testing.T) {
	spec := Spec{
		Chains: []ChainSpec{
			AttachedEVM{ID: "chain-a", EVMChainID: 1, Endpoint: "a", Timing: testTiming()},
			AttachedEVM{ID: "chain-b", EVMChainID: 2, Endpoint: "b", Timing: testTiming()},
			AttachedEVM{ID: "chain-c", EVMChainID: 3, Endpoint: "c", Timing: testTiming()},
		},
		IBCInstances: []IBCInstanceSpec{
			ExistingIBCInstance{ID: "ibc-a", Chain: "chain-a", Locator: "0x1"},
			ExistingIBCInstance{ID: "ibc-b", Chain: "chain-b", Locator: "0x2"},
			ExistingIBCInstance{ID: "ibc-c", Chain: "chain-c", Locator: "0x3"},
		},
		Connections: []ConnectionSpec{
			{
				ID: "ab",
				A:  ExistingClient{IBCInstance: "ibc-a", Locator: "shared"},
				B:  ExistingClient{IBCInstance: "ibc-b", Locator: "b"},
			},
			{
				ID: "ac",
				A:  ExistingClient{IBCInstance: "ibc-a", Locator: "shared"},
				B:  ExistingClient{IBCInstance: "ibc-c", Locator: "c"},
			},
		},
	}
	require.ErrorContains(
		t,
		spec.validate(),
		`IBC Clients "ab/A" and "ac/A" on IBC Instance "ibc-a" resolve to duplicate locator "shared"`,
	)

	spec.Connections[1].A = ExistingClient{IBCInstance: "ibc-c", Locator: "shared"}
	spec.Connections[1].B = ExistingClient{IBCInstance: "ibc-a", Locator: "a-second"}
	require.NoError(t, spec.validate(), "the same locator is legal on distinct IBC Instances")
}

func TestSpecValidateRejectsExistingInstanceAliases(t *testing.T) {
	spec := validSpec()
	makeChainAAttached(&spec)
	spec.IBCInstances = []IBCInstanceSpec{
		ExistingIBCInstance{ID: "ibc-a", Chain: "chain-a", Locator: "0xrouter"},
		ExistingIBCInstance{ID: "ibc-b", Chain: "chain-b", Locator: "0xother"},
		ExistingIBCInstance{ID: "ibc-a-alias", Chain: "chain-a", Locator: "0xrouter"},
	}
	require.ErrorContains(
		t,
		spec.validate(),
		`existing IBC Instances "ibc-a" and "ibc-a-alias" on Chain "chain-a" reference duplicate locator "0xrouter"`,
	)
}

func TestSpecValidateRejectsPointerClientDeclaration(t *testing.T) {
	spec := validSpec()
	client := spec.Connections[0].A.(NewClient)
	spec.Connections[0].A = &client
	require.ErrorContains(
		t,
		spec.validate(),
		"unsupported declaration *environment.NewClient; use a concrete value",
	)
}

func TestSpecValidateDoesNotMutate(t *testing.T) {
	spec := validSpec()
	want := validSpec()
	require.NoError(t, spec.validate())
	require.Equal(t, want, spec)
}

func TestSpecSnapshotOwnsCollections(t *testing.T) {
	spec := validSpec()
	snapshot := spec.snapshot()

	spec.Chains[0] = ManagedAnvil{ID: "changed", EVMChainID: 8}
	spec.IBCInstances[0] = ExistingIBCInstance{ID: "changed"}
	spec.Connections[0] = ConnectionSpec{ID: "changed"}
	spec.Attestors[0].ID = "changed"

	require.Equal(t, ChainID("chain-a"), snapshot.Chains[0].chainID())
	require.Equal(t, IBCInstanceID("ibc-a"), snapshot.IBCInstances[0].ibcInstanceID())
	require.Equal(t, ConnectionID("connection-ab"), snapshot.Connections[0].ID)
	require.Equal(t, AttestorID("attestor-1"), snapshot.Attestors[0].ID)
}

func validSpec() Spec {
	return Spec{
		Chains: []ChainSpec{
			ManagedAnvil{ID: "chain-a", EVMChainID: 31337},
			AttachedEVM{
				ID:         "chain-b",
				EVMChainID: 31338,
				Endpoint:   "chain-b-rpc",
				Timing: Timing{
					BlockInterval:    2 * time.Second,
					CompletionBudget: 40 * time.Second,
					PollInterval:     250 * time.Millisecond,
				},
			},
		},
		IBCInstances: []IBCInstanceSpec{
			NewIBCInstance{ID: "ibc-a", Chain: "chain-a", Authority: "deploy-a"},
			ExistingIBCInstance{ID: "ibc-b", Chain: "chain-b", Locator: "0xibc-b"},
		},
		Connections: []ConnectionSpec{
			{
				ID: "connection-ab",
				A: NewClient{
					IBCInstance: "ibc-a", Authority: "connect-a", MinRequiredSignatures: 1,
				},
				B: NewClient{
					IBCInstance: "ibc-b", Authority: "connect-b", MinRequiredSignatures: 1,
				},
			},
		},
		Attestors: []AttestorSpec{
			{
				ID: "attestor-1", Client: IBCClientRef{
					Connection: "connection-ab", End: ConnectionEndA,
				}, Authority: "attest-a",
			},
			{
				ID: "attestor-2", Client: IBCClientRef{
					Connection: "connection-ab", End: ConnectionEndB,
				}, Authority: "attest-b",
			},
		},
	}
}

func existingConnectionSpec() ConnectionSpec {
	return ConnectionSpec{
		ID: "connection-ab",
		A:  ExistingClient{IBCInstance: "ibc-a", Locator: "client-7"},
		B:  ExistingClient{IBCInstance: "ibc-b", Locator: "client-9"},
	}
}

func makeChainAAttached(spec *Spec) {
	spec.Chains[0] = AttachedEVM{
		ID:         "chain-a",
		EVMChainID: 31337,
		Endpoint:   "chain-a-rpc",
		Timing:     spec.Chains[1].(AttachedEVM).Timing,
	}
}
