package environment

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/harness/internal/eureka"
)

func TestStartRealizesMixedProtocolGraphWithInjectedDrivers(t *testing.T) {
	spec := mixedProtocolSpec()
	runtime := mixedProtocolRuntime()
	attestorAccount, err := runtime.evmAccount("attestor-signer")
	require.NoError(t, err)

	var (
		chainsReady     atomic.Int32
		instancesReady  atomic.Int32
		connectionReady atomic.Bool
		releaseMu       sync.Mutex
		releases        []string
	)
	recordRelease := func(resource string) {
		releaseMu.Lock()
		defer releaseMu.Unlock()
		releases = append(releases, resource)
	}

	env, err := start(t.Context(), spec, runtime, drivers{
		acquireChain: func(_ context.Context, declaration ChainSpec, _ Runtime, _ workspace) (chainAcquisition, error) {
			chainsReady.Add(1)
			id := declaration.chainID()
			return fakeAcquisition(id, OwnershipOwnedEphemeral, CleanupActionStop, func(context.Context) error {
				recordRelease("chain:" + string(id))
				return nil
			}), nil
		},
		acquireIBCInstance: func(
			_ context.Context,
			declaration IBCInstanceSpec,
			chain *Chain,
			_ Runtime,
		) (instanceAcquisition, error) {
			require.EqualValues(t, 2, chainsReady.Load(), "both Chains are ready before protocol setup")
			require.Equal(t, declaration.ibcInstanceChain(), chain.ID())
			instancesReady.Add(1)

			switch instance := declaration.(type) {
			case NewIBCInstance:
				return instanceAcquisition{
					instance:  &IBCInstance{id: instance.ID, chain: chain, locator: "router-a"},
					ownership: OwnershipOwnedDurable,
					receipt: &IBCInstanceReceipt{
						ID:    instance.ID,
						Chain: instance.Chain,
						AccessManager: successfulTestTransaction(
							"0xinstance",
							"0x1000000000000000000000000000000000000001",
						),
					},
				}, nil
			case ExistingIBCInstance:
				return instanceAcquisition{
					instance:  &IBCInstance{id: instance.ID, chain: chain, locator: instance.Locator},
					ownership: OwnershipBorrowed,
				}, nil
			default:
				return instanceAcquisition{}, errors.New("unexpected IBC Instance declaration")
			}
		},
		acquireConnection: func(
			_ context.Context,
			declaration ConnectionSpec,
			dependencies connectionDependencies,
			_ Runtime,
		) (connectionAcquisition, error) {
			require.EqualValues(t, 2, instancesReady.Load(), "both IBC Instances are ready before Clients")
			require.IsType(t, NewClient{}, declaration.A)
			require.IsType(t, ExistingClient{}, declaration.B)
			require.Equal(t, []AttestorSpec{spec.Attestors[0]}, dependencies.attestorSpecs["client-a"])
			require.Empty(t, dependencies.attestorSpecs["client-b"])

			connection := resolvedMixedConnection(
				declaration.ID,
				dependencies.instances,
				EVMAddress(attestorAccount.Address().Hex()),
			)
			connectionReady.Store(true)
			receipt := &IBCConnectionReceipt{
				ID: declaration.ID,
				A: &IBCClientReceipt{
					ID:          "client-a",
					IBCInstance: "ibc-a",
					Locator:     "client-a-onchain",
					LightClientDeployment: successfulTestTransaction(
						"0xclient-a",
						"0x2000000000000000000000000000000000000002",
					),
				},
				B: &IBCClientReceipt{ID: "client-b", IBCInstance: "ibc-b", Locator: "existing-client-b"},
			}
			return connectionAcquisition{
				connection: connection,
				ownership:  OwnershipOwnedDurable,
				receipt:    receipt,
				a: clientAcquisition{
					client: connection.a, ownership: OwnershipOwnedDurable, receipt: receipt.A, attempted: true,
				},
				b: clientAcquisition{
					client: connection.b, ownership: OwnershipBorrowed, receipt: receipt.B, attempted: true,
				},
			}, nil
		},
		acquireAttestor: func(
			_ context.Context,
			declaration AttestorSpec,
			dependencies attestorDependencies,
			_ Runtime,
			_ workspace,
		) (attestorAcquisition, error) {
			require.True(t, connectionReady.Load(), "the reciprocal Client pair is ready before its Attestor")
			require.Equal(t, ClientID("client-a"), dependencies.client.ID())
			require.Equal(t, IBCInstanceID("ibc-b"), dependencies.observed.ID())
			return attestorAcquisition{
				attestor: &Attestor{
					id: declaration.ID, client: dependencies.client, observed: dependencies.observed,
					signer: EVMAddress(attestorAccount.Address().Hex()),
				},
				ownership: OwnershipOwnedEphemeral,
				action:    CleanupActionStop,
				release: func(context.Context) error {
					recordRelease("attestor:" + string(declaration.ID))
					return nil
				},
			}, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, env)

	connection, err := env.Connection("connection-ab")
	require.NoError(t, err)
	require.Equal(t, IBCClientLocator("existing-client-b"), connection.A().CounterpartyLocator())
	require.Equal(t, IBCClientLocator("client-a-onchain"), connection.B().CounterpartyLocator())
	require.Same(t, connection.A(), mustIBCClient(t, env, "client-a"))
	require.Same(t, connection.B(), mustIBCClient(t, env, "client-b"))

	attestor, err := env.Attestor("attestor-a")
	require.NoError(t, err)
	require.Same(t, connection.A(), attestor.IBCClient())
	require.Equal(t, IBCInstanceID("ibc-b"), attestor.ObservedIBCInstance().ID())
	require.Equal(t, EVMAddress(attestorAccount.Address().Hex()), attestor.SignerAddress())

	require.NoError(t, env.Close(t.Context()))
	releaseMu.Lock()
	gotReleases := append([]string(nil), releases...)
	releaseMu.Unlock()
	require.Len(t, gotReleases, 3)
	require.Equal(t, "attestor:attestor-a", gotReleases[0], "Attestors stop before their Chains")
	require.ElementsMatch(t, []string{"chain:chain-a", "chain:chain-b"}, gotReleases[1:])

	resources := env.Manifest().Resources()
	require.Equal(t, ResourceRecord{
		Kind: ResourceKindIBCInstance, ID: "ibc-a", Ownership: OwnershipOwnedDurable, State: ResourceStateRetained,
	}, resourceByID(t, resources, "ibc-a"))
	require.Equal(t, ResourceRecord{
		Kind: ResourceKindIBCInstance, ID: "ibc-b", Ownership: OwnershipBorrowed, State: ResourceStateRetained,
	}, resourceByID(t, resources, "ibc-b"))
	require.Equal(t, ResourceRecord{
		Kind:      ResourceKindIBCConnection,
		ID:        "connection-ab",
		Ownership: OwnershipOwnedDurable,
		State:     ResourceStateRetained,
	}, resourceByID(t, resources, "connection-ab"))
	require.Equal(t, ResourceRecord{
		Kind: ResourceKindIBCClient, ID: "client-a", Ownership: OwnershipOwnedDurable, State: ResourceStateRetained,
	}, resourceByID(t, resources, "client-a"))
	require.Equal(t, ResourceRecord{
		Kind: ResourceKindIBCClient, ID: "client-b", Ownership: OwnershipBorrowed, State: ResourceStateRetained,
	}, resourceByID(t, resources, "client-b"))
	require.Equal(t, ResourceRecord{
		Kind: ResourceKindAttestor, ID: "attestor-a", Ownership: OwnershipOwnedEphemeral, State: ResourceStateReleased,
	}, resourceByID(t, resources, "attestor-a"))
}

func TestStartConnectionFailurePreservesPartialReceiptsAndOwnership(t *testing.T) {
	spec := mixedProtocolSpec()
	runtime := mixedProtocolRuntime()
	connectionErr := errors.New("client B setup failed with adapter detail")
	partial := IBCConnectionReceipt{
		ID: "connection-ab",
		A: &IBCClientReceipt{
			ID: "client-a", IBCInstance: "ibc-a", Locator: "client-a-onchain",
			LightClientAddress: "0x2000000000000000000000000000000000000002",
			LightClientDeployment: successfulTestTransaction(
				"0xclient-a",
				"0x2000000000000000000000000000000000000002",
			),
		},
	}
	instanceReceipt := IBCInstanceReceipt{
		ID: "ibc-a", Chain: "chain-a",
		AccessManager: successfulTestTransaction("0xinstance", "0x1000000000000000000000000000000000000001"),
	}
	var (
		chainReleases  atomic.Int32
		attestorCalled atomic.Bool
	)

	env, err := start(t.Context(), spec, runtime, drivers{
		acquireChain: func(_ context.Context, declaration ChainSpec, _ Runtime, _ workspace) (chainAcquisition, error) {
			return fakeAcquisition(
				declaration.chainID(),
				OwnershipOwnedEphemeral,
				CleanupActionStop,
				func(context.Context) error {
					chainReleases.Add(1)
					return nil
				},
			), nil
		},
		acquireIBCInstance: func(
			_ context.Context,
			declaration IBCInstanceSpec,
			chain *Chain,
			_ Runtime,
		) (instanceAcquisition, error) {
			switch instance := declaration.(type) {
			case NewIBCInstance:
				return instanceAcquisition{
					instance:  &IBCInstance{id: instance.ID, chain: chain, locator: "router-a"},
					ownership: OwnershipOwnedDurable,
					receipt:   &instanceReceipt,
				}, nil
			case ExistingIBCInstance:
				return instanceAcquisition{
					instance:  &IBCInstance{id: instance.ID, chain: chain, locator: instance.Locator},
					ownership: OwnershipBorrowed,
				}, nil
			default:
				return instanceAcquisition{}, errors.New("unexpected IBC Instance declaration")
			}
		},
		acquireConnection: func(
			_ context.Context,
			_ ConnectionSpec,
			dependencies connectionDependencies,
			_ Runtime,
		) (connectionAcquisition, error) {
			a := &IBCClient{
				id: "client-a", instance: dependencies.instances["ibc-a"], locator: "client-a-onchain",
				lightClient: partial.A.LightClientAddress, counterparty: "existing-client-b",
			}
			return connectionAcquisition{
				ownership: OwnershipOwnedDurable,
				receipt:   &partial,
				a: clientAcquisition{
					client: a, ownership: OwnershipOwnedDurable, receipt: partial.A, attempted: true,
				},
				b: clientAcquisition{ownership: OwnershipBorrowed, attempted: true},
			}, connectionErr
		},
		acquireAttestor: func(
			context.Context,
			AttestorSpec,
			attestorDependencies,
			Runtime,
			workspace,
		) (attestorAcquisition, error) {
			attestorCalled.Store(true)
			return attestorAcquisition{}, nil
		},
	})
	require.Nil(t, env)
	var startErr *StartError
	require.ErrorAs(t, err, &startErr)
	require.ErrorIs(t, err, connectionErr)
	require.Contains(t, err.Error(), "adapter detail")
	require.Equal(t, []FailureRecord{{
		Kind: ResourceKindIBCConnection, ID: "connection-ab",
	}}, startErr.Failures())
	require.EqualValues(t, 2, chainReleases.Load())
	require.False(t, attestorCalled.Load(), "dependent Attestors do not start after a Connection failure")

	require.Equal(t, []IBCInstanceReceipt{instanceReceipt}, startErr.IBCInstanceReceipts())
	require.Equal(t, []IBCConnectionReceipt{partial}, startErr.IBCConnectionReceipts())
	resources := startErr.Manifest().Resources()
	require.Equal(t, ResourceRecord{
		Kind: ResourceKindIBCInstance, ID: "ibc-a", Ownership: OwnershipOwnedDurable, State: ResourceStateRetained,
	}, resourceByID(t, resources, "ibc-a"))
	require.Equal(t, ResourceRecord{
		Kind: ResourceKindIBCInstance, ID: "ibc-b", Ownership: OwnershipBorrowed, State: ResourceStateRetained,
	}, resourceByID(t, resources, "ibc-b"))
	require.Equal(t, ResourceRecord{
		Kind:      ResourceKindIBCConnection,
		ID:        "connection-ab",
		Ownership: OwnershipOwnedDurable,
		State:     ResourceStateFailed,
	}, resourceByID(t, resources, "connection-ab"))
	require.Equal(t, ResourceRecord{
		Kind: ResourceKindIBCClient, ID: "client-a", Ownership: OwnershipOwnedDurable, State: ResourceStateRetained,
	}, resourceByID(t, resources, "client-a"))
	for _, chainID := range []string{"chain-a", "chain-b"} {
		require.Equal(t, ResourceRecord{
			Kind: ResourceKindChain, ID: chainID, Ownership: OwnershipOwnedEphemeral, State: ResourceStateReleased,
		}, resourceByID(t, resources, chainID))
	}
}

func TestPreparationFailurePreservesResolvedExistingClientEvidence(t *testing.T) {
	prepareErr := errors.New("later preparation failed")
	spec := Spec{Connections: []ConnectionSpec{{
		ID: "connection-ab",
		A: ExistingClient{
			ID: "client-a", IBCInstance: "ibc-a", Locator: "existing-a",
		},
		B: NewClient{
			ID: "client-b", IBCInstance: "ibc-b",
		},
	}}}
	instances := map[IBCInstanceID]*IBCInstance{
		"ibc-a": {id: "ibc-a"},
		"ibc-b": {id: "ibc-b"},
	}
	resolvedA := &IBCClient{
		id:          "client-a",
		instance:    instances["ibc-a"],
		locator:     "existing-a",
		lightClient: "0x1000000000000000000000000000000000000001",
	}
	receipts := newProtocolReceiptJournal()

	_, _, failures, err := acquireConnections(
		t.Context(),
		spec,
		instances,
		Runtime{},
		drivers{prepareConnections: func(
			context.Context,
			Spec,
			connectionDependencies,
			Runtime,
		) (connectionDependencies, []FailureRecord, error) {
			failure := FailureRecord{
				Kind: ResourceKindIBCClient, ID: "client-b",
			}
			return connectionDependencies{
				existingClients: map[ClientID]*IBCClient{"client-a": resolvedA},
			}, []FailureRecord{failure}, prepareErr
		}},
		newJournal(),
		receipts,
	)
	require.ErrorIs(t, err, prepareErr)
	require.Equal(t, []FailureRecord{{
		Kind: ResourceKindIBCClient, ID: "client-b",
	}}, failures)
	require.Equal(t, []IBCConnectionReceipt{{
		ID: "connection-ab",
		A: &IBCClientReceipt{
			ID:                 "client-a",
			IBCInstance:        "ibc-a",
			Locator:            "existing-a",
			LightClientAddress: "0x1000000000000000000000000000000000000001",
		},
	}}, receipts.snapshot().connections)
}

func TestClientEvidencePreservesPredictedAddressAfterAmbiguousBroadcast(t *testing.T) {
	predicted := common.HexToAddress("0x1000000000000000000000000000000000000001")
	out := &IBCClientReceipt{}
	translateClientReceipts(out, eureka.ClientReceipts{LightClient: &eureka.TransactionEvidence{
		Hash:                     common.HexToHash("0x1234"),
		Submission:               eureka.TransactionSubmissionAmbiguous,
		PredictedContractAddress: predicted,
	}})

	require.Equal(t, EVMAddress(predicted.Hex()), out.LightClientAddress)
	require.Equal(t, EVMSubmissionAmbiguous, out.LightClientDeployment.Submission)
	require.Equal(t, EVMAddress(predicted.Hex()), out.LightClientDeployment.PredictedContractAddress)
	require.Empty(t, out.LightClientDeployment.ContractAddress)
	require.False(t, out.LightClientDeployment.Mined)
}

func TestStartPreservesUnminedTransactionAndReleasesHostScopedState(t *testing.T) {
	spec := Spec{
		Chains: []ChainSpec{ManagedAnvil{ID: "chain-a", EVMChainID: 31337}},
		IBCInstances: []IBCInstanceSpec{
			NewIBCInstance{ID: "ibc-a", Chain: "chain-a", Authority: "owner"},
		},
	}
	runtime := Runtime{Authorities: map[AuthorityID]EVMAuthority{
		"owner": {PrivateKeyHex: testPrimaryPrivateKeyHex},
	}}
	pending := IBCInstanceReceipt{
		ID: "ibc-a", Chain: "chain-a",
		AccessManager: &EVMTransactionEvidence{
			Hash: "0xpending", Submission: EVMSubmissionAccepted, Mined: false,
		},
	}

	_, err := start(t.Context(), spec, runtime, drivers{
		acquireChain: func(_ context.Context, declaration ChainSpec, _ Runtime, _ workspace) (chainAcquisition, error) {
			return fakeAcquisition(
				declaration.chainID(),
				OwnershipOwnedEphemeral,
				CleanupActionStop,
				func(context.Context) error { return nil },
			), nil
		},
		acquireIBCInstance: func(
			context.Context,
			IBCInstanceSpec,
			*Chain,
			Runtime,
		) (instanceAcquisition, error) {
			return instanceAcquisition{
				ownership: OwnershipOwnedHostScoped,
				receipt:   &pending,
			}, context.DeadlineExceeded
		},
	})
	var startErr *StartError
	require.ErrorAs(t, err, &startErr)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, []IBCInstanceReceipt{pending}, startErr.IBCInstanceReceipts())
	require.Equal(t, ResourceRecord{
		Kind: ResourceKindIBCInstance, ID: "ibc-a", Ownership: OwnershipOwnedHostScoped, State: ResourceStateReleased,
	}, resourceByID(t, startErr.Manifest().Resources(), "ibc-a"))
}

func TestResolvedClientsRejectAttestorSignerReuse(t *testing.T) {
	const signer EVMAddress = "0x1000000000000000000000000000000000000001"
	seen := make(map[EVMAddress]ClientID)
	require.NoError(t, recordResolvedAttestorUse(seen, &IBCClient{
		id: "client-a", attestors: []EVMAddress{signer},
	}))
	err := recordResolvedAttestorUse(seen, &IBCClient{
		id: "client-b", attestors: []EVMAddress{signer},
	})
	require.ErrorContains(
		t,
		err,
		`attestor signer 0x1000000000000000000000000000000000000001 is reused by resolved IBC Clients "client-a" and "client-b"`,
	)
}

func TestExistingClientRequiresDeclaredAttestorSignerMembership(t *testing.T) {
	runtime := Runtime{Authorities: map[AuthorityID]EVMAuthority{
		"registered": {PrivateKeyHex: testPrimaryPrivateKeyHex},
		"absent":     {PrivateKeyHex: testSecondaryPrivateKeyHex},
	}}
	registered, err := runtime.evmAccount("registered")
	require.NoError(t, err)
	onChain := []common.Address{registered.Address()}

	require.NoError(t, requireDeclaredAttestors(
		"client-a",
		onChain,
		[]AttestorSpec{{ID: "attestor-a", Client: "client-a", Authority: "registered"}},
		runtime,
	))
	err = requireDeclaredAttestors(
		"client-a",
		onChain,
		[]AttestorSpec{{ID: "attestor-a", Client: "client-a", Authority: "absent"}},
		runtime,
	)
	require.ErrorContains(t, err, `Attestor "attestor-a" signer`)
	require.ErrorContains(t, err, `is not registered for existing IBC Client "client-a"`)
}

func TestFailedAttestorStartJournalsPartialProcessForCleanup(t *testing.T) {
	spec := mixedProtocolSpec()
	instances := map[IBCInstanceID]*IBCInstance{
		"ibc-a": {id: "ibc-a"},
		"ibc-b": {id: "ibc-b"},
	}
	clients := map[ClientID]*IBCClient{
		"client-a": {id: "client-a", instance: instances["ibc-a"]},
	}
	resources := newJournal()
	effects := &effectJournal{}
	var releases atomic.Int32
	startErr := errors.New("readiness failed")

	_, failures, err := acquireAttestors(
		t.Context(),
		spec,
		instances,
		clients,
		mixedProtocolRuntime(),
		workspace{privateDir: t.TempDir(), diagnosticsDir: t.TempDir()},
		drivers{acquireAttestor: func(
			context.Context,
			AttestorSpec,
			attestorDependencies,
			Runtime,
			workspace,
		) (attestorAcquisition, error) {
			return attestorAcquisition{
				ownership: OwnershipOwnedEphemeral,
				action:    CleanupActionStop,
				release: func(context.Context) error {
					releases.Add(1)
					return nil
				},
			}, startErr
		}},
		resources,
		effects,
	)
	require.ErrorIs(t, err, startErr)
	require.Equal(t, []FailureRecord{{
		Kind: ResourceKindAttestor, ID: "attestor-a",
	}}, failures)
	require.Equal(t, ResourceStateFailed, resourceByID(t, resources.snapshot().Resources(), "attestor-a").State)

	require.Empty(t, effects.cleanup(t.Context(), resources))
	require.EqualValues(t, 1, releases.Load())
	require.Equal(t, ResourceStateReleased, resourceByID(t, resources.snapshot().Resources(), "attestor-a").State)
}

func TestProtocolDeclarationsUseCanonicalIdentityOrder(t *testing.T) {
	instances := sortedIBCInstanceSpecs([]IBCInstanceSpec{
		ExistingIBCInstance{ID: "z"},
		ExistingIBCInstance{ID: "a"},
	})
	require.Equal(t, []IBCInstanceID{"a", "z"}, []IBCInstanceID{
		instances[0].ibcInstanceID(), instances[1].ibcInstanceID(),
	})

	connections := sortedConnectionSpecs([]ConnectionSpec{{ID: "z"}, {ID: "a"}})
	require.Equal(t, []ConnectionID{"a", "z"}, []ConnectionID{connections[0].ID, connections[1].ID})

	attestors := sortedAttestorSpecs([]AttestorSpec{{ID: "z"}, {ID: "a"}})
	require.Equal(t, []AttestorID{"a", "z"}, []AttestorID{attestors[0].ID, attestors[1].ID})
}

func mixedProtocolSpec() Spec {
	return Spec{
		Chains: []ChainSpec{
			ManagedAnvil{ID: "chain-a", EVMChainID: 31337},
			AttachedEVM{
				ID: "chain-b", EVMChainID: 31338, Endpoint: "chain-b-rpc", Timing: testTiming(),
			},
		},
		IBCInstances: []IBCInstanceSpec{
			NewIBCInstance{ID: "ibc-a", Chain: "chain-a", Authority: "instance-owner"},
			ExistingIBCInstance{ID: "ibc-b", Chain: "chain-b", Locator: "0x1000000000000000000000000000000000000001"},
		},
		Connections: []ConnectionSpec{{
			ID: "connection-ab",
			A: NewClient{
				ID: "client-a", IBCInstance: "ibc-a", Authority: "client-owner", MinRequiredSignatures: 1,
			},
			B: ExistingClient{
				ID: "client-b", IBCInstance: "ibc-b", Locator: "existing-client-b",
			},
		}},
		Attestors: []AttestorSpec{{
			ID: "attestor-a", Client: "client-a", Authority: "attestor-signer",
		}},
	}
}

func mixedProtocolRuntime() Runtime {
	return Runtime{
		Endpoints: map[EndpointBindingID]EndpointBinding{
			"chain-b-rpc": {RPCURL: "http://127.0.0.1:8546"},
		},
		Authorities: map[AuthorityID]EVMAuthority{
			"instance-owner":  {PrivateKeyHex: testPrimaryPrivateKeyHex},
			"client-owner":    {PrivateKeyHex: testPrimaryPrivateKeyHex},
			"attestor-signer": {PrivateKeyHex: testSecondaryPrivateKeyHex},
		},
	}
}

func resolvedMixedConnection(
	id ConnectionID,
	instances map[IBCInstanceID]*IBCInstance,
	attestor EVMAddress,
) *Connection {
	a := &IBCClient{
		id: "client-a", instance: instances["ibc-a"], locator: "client-a-onchain",
		lightClient: "0x2000000000000000000000000000000000000002", counterparty: "existing-client-b",
		attestors: []EVMAddress{attestor}, minRequiredSignatures: 1,
	}
	b := &IBCClient{
		id: "client-b", instance: instances["ibc-b"], locator: "existing-client-b",
		lightClient: "0x3000000000000000000000000000000000000003", counterparty: "client-a-onchain",
		minRequiredSignatures: 1,
	}
	return &Connection{id: id, a: a, b: b}
}

func successfulTestTransaction(hash string, contract EVMAddress) *EVMTransactionEvidence {
	return &EVMTransactionEvidence{
		Hash: hash, Submission: EVMSubmissionAccepted, Mined: true,
		BlockNumber: 7, Status: 1, PredictedContractAddress: contract, ContractAddress: contract,
	}
}

func mustIBCClient(t *testing.T, env *Environment, id ClientID) *IBCClient {
	t.Helper()
	client, err := env.IBCClient(id)
	require.NoError(t, err)
	return client
}
