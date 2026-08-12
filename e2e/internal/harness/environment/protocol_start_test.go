// SPDX-License-Identifier: Apache-2.0

package environment

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnvironmentCloseWaitsForLeasedAttestorUse(t *testing.T) {
	lease := &environmentLease{}
	attestor := &Attestor{id: "attestor-a"}
	attestor.bindLease(lease)
	cleanupStarted := make(chan struct{})
	env := &Environment{
		effects: &effectJournal{effects: []cleanupEffect{{
			description: "stop Attestor attestor-a",
			release: func(context.Context) error {
				close(cleanupStarted)
				return nil
			},
		}}},
		lease: lease,
	}

	useStarted := make(chan struct{})
	releaseUse := make(chan struct{})
	var releaseUseOnce sync.Once
	useDone := make(chan error, 1)
	useFinished := make(chan struct{})
	closeDone := make(chan error, 1)
	closeFinished := make(chan struct{})
	closeStarted := false
	go func() {
		useDone <- attestor.use(func() error {
			close(useStarted)
			<-releaseUse
			return nil
		})
		close(useFinished)
	}()
	defer func() {
		releaseUseOnce.Do(func() { close(releaseUse) })
		select {
		case <-useFinished:
		case <-time.After(time.Second):
			assert.Fail(t, "leased Attestor use did not finish")
		}
		if closeStarted {
			select {
			case <-closeFinished:
			case <-time.After(time.Second):
				assert.Fail(t, "Environment.Close did not finish")
			}
		}
	}()

	select {
	case <-useStarted:
	case <-time.After(time.Second):
		require.FailNow(t, "leased Attestor use did not start")
	}
	closeStarted = true
	go func() {
		closeDone <- env.Close(context.Background())
		close(closeFinished)
	}()

	require.Eventually(t, func() bool {
		return errors.Is(attestor.use(func() error { return nil }), ErrEnvironmentClosed)
	}, time.Second, time.Millisecond)
	select {
	case <-cleanupStarted:
		require.FailNow(t, "Attestor cleanup started before the leased use finished")
	default:
	}

	releaseUseOnce.Do(func() { close(releaseUse) })
	select {
	case err := <-useDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(t, "leased Attestor use did not finish")
	}
	select {
	case err := <-closeDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(t, "Environment.Close did not finish")
	}
	select {
	case <-cleanupStarted:
	default:
		require.FailNow(t, "Attestor cleanup did not run")
	}
	require.ErrorIs(t, attestor.use(func() error { return nil }), ErrEnvironmentClosed)
}

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
			return fakeAcquisition(id, func(context.Context) error {
				recordRelease("chain:" + string(id))
				return nil
			}), nil
		},
		acquireIBCInstance: func(_ context.Context, declaration IBCInstanceSpec, chain *Chain, _ Runtime) (*IBCInstance, error) {
			require.EqualValues(t, 2, chainsReady.Load())
			require.Equal(t, declaration.ibcInstanceChain(), chain.ID())
			instancesReady.Add(1)
			switch instance := declaration.(type) {
			case NewIBCInstance:
				return &IBCInstance{id: instance.ID, chain: chain, locator: "router-a"}, nil
			case ExistingIBCInstance:
				return &IBCInstance{id: instance.ID, chain: chain, locator: instance.Locator}, nil
			default:
				return nil, errors.New("unexpected IBC Instance declaration")
			}
		},
		acquireConnection: func(_ context.Context, declaration ConnectionSpec, dependencies connectionDependencies, _ Runtime) (*Connection, error) {
			require.EqualValues(t, 2, instancesReady.Load())
			connectionReady.Store(true)
			return resolvedMixedConnection(
				declaration.ID,
				dependencies.instances,
				EVMAddress(attestorAccount.Address().Hex()),
			), nil
		},
		acquireAttestor: func(_ context.Context, declaration AttestorSpec, dependencies attestorDependencies, _ Runtime, _ workspace) (attestorAcquisition, error) {
			require.True(t, connectionReady.Load())
			require.Equal(t, "client-a-onchain", dependencies.client.ID())
			require.Equal(t, IBCInstanceID("ibc-b"), dependencies.observed.ID())
			return attestorAcquisition{
				attestor: &Attestor{
					id: declaration.ID, client: dependencies.client, observed: dependencies.observed,
					signer: EVMAddress(attestorAccount.Address().Hex()),
				},
				description: "stop Attestor " + string(declaration.ID),
				release: func(context.Context) error {
					recordRelease("attestor:" + string(declaration.ID))
					return nil
				},
			}, nil
		},
	})
	require.NoError(t, err)

	connection, err := env.Connection("connection-ab")
	require.NoError(t, err)
	require.Equal(t, "existing-client-b", connection.A().CounterpartyID())

	attestor, err := env.Attestor("attestor-a")
	require.NoError(t, err)
	require.Same(t, connection.A(), attestor.IBCClient())
	require.Equal(t, IBCInstanceID("ibc-b"), attestor.ObservedIBCInstance().ID())

	require.NoError(t, env.Close(t.Context()))
	releaseMu.Lock()
	gotReleases := append([]string(nil), releases...)
	releaseMu.Unlock()
	require.Equal(t, "attestor:attestor-a", gotReleases[0])
	require.ElementsMatch(t, []string{"chain:chain-a", "chain:chain-b"}, gotReleases[1:])
}

func TestConnectionFailureStopsBeforeAttestorsAndCleansChains(t *testing.T) {
	spec := mixedProtocolSpec()
	connectionErr := errors.New("connection failed")
	var releases atomic.Int32
	attestorStarted := false

	env, err := start(t.Context(), spec, mixedProtocolRuntime(), drivers{
		acquireChain: func(_ context.Context, declaration ChainSpec, _ Runtime, _ workspace) (chainAcquisition, error) {
			return fakeAcquisition(declaration.chainID(), func(context.Context) error {
				releases.Add(1)
				return nil
			}), nil
		},
		acquireIBCInstance: func(_ context.Context, declaration IBCInstanceSpec, chain *Chain, _ Runtime) (*IBCInstance, error) {
			return &IBCInstance{
				id: declaration.ibcInstanceID(), chain: chain, locator: "router",
			}, nil
		},
		acquireConnection: func(context.Context, ConnectionSpec, connectionDependencies, Runtime) (*Connection, error) {
			return nil, connectionErr
		},
		acquireAttestor: func(context.Context, AttestorSpec, attestorDependencies, Runtime, workspace) (attestorAcquisition, error) {
			attestorStarted = true
			return attestorAcquisition{}, nil
		},
	})

	require.Nil(t, env)
	require.ErrorIs(t, err, connectionErr)
	require.False(t, attestorStarted)
	require.EqualValues(t, len(spec.Chains), releases.Load())
}

func TestResolvedClientsRejectAttestorSignerReuse(t *testing.T) {
	const signer EVMAddress = "0x1000000000000000000000000000000000000001"
	seen := make(map[EVMAddress]string)
	require.NoError(t, recordResolvedAttestorUse(seen, &IBCClient{
		label: "one/A", attestors: []EVMAddress{signer},
	}))
	err := recordResolvedAttestorUse(seen, &IBCClient{
		label: "two/A", attestors: []EVMAddress{signer},
	})
	require.ErrorContains(t, err, `attestor signer 0x1000000000000000000000000000000000000001 is reused`)
}

func TestPreparedClientsRejectExistingSignerReuseBeforeDeployment(t *testing.T) {
	runtime := Runtime{Authorities: map[AuthorityID]EVMAuthority{
		"new-signer": {PrivateKeyHex: testPrimaryPrivateKeyHex},
	}}
	account, err := runtime.evmAccount("new-signer")
	require.NoError(t, err)
	spec := Spec{Connections: []ConnectionSpec{{
		ID: "connection-ab",
		A:  ExistingClient{ID: "existing", IBCInstance: "ibc-a"},
		B: NewClient{
			IBCInstance: "ibc-b", Attestors: []AttestorSpec{{ID: "new", Authority: "new-signer"}},
		},
	}}}
	dependencies := connectionDependencies{existingClients: map[string]*IBCClient{
		"connection-ab/A": {
			label: "connection-ab/A", attestors: []EVMAddress{EVMAddress(account.Address().Hex())},
		},
	}}

	err = validatePreparedAttestorUse(spec, dependencies, runtime)
	require.ErrorContains(t, err, "is reused by resolved IBC Clients")
}

func TestExistingClientRequiresDeclaredAttestorSignerMembership(t *testing.T) {
	runtime := Runtime{Authorities: map[AuthorityID]EVMAuthority{
		"registered": {PrivateKeyHex: testPrimaryPrivateKeyHex},
		"absent":     {PrivateKeyHex: testSecondaryPrivateKeyHex},
	}}
	registered, err := runtime.evmAccount("registered")
	require.NoError(t, err)
	onChain := []common.Address{registered.Address()}

	label := "connection-ab/A"
	require.NoError(t, requireDeclaredAttestors(label, onChain, []AttestorSpec{{
		ID: "attestor-a", Authority: "registered",
	}}, runtime))
	err = requireDeclaredAttestors(label, onChain, []AttestorSpec{{
		ID: "attestor-a", Authority: "absent",
	}}, runtime)
	require.ErrorContains(t, err, `is not registered for existing IBC Client "connection-ab/A"`)
}

func TestClientIDUsesStableConnectionEnd(t *testing.T) {
	client := NewClient{IBCInstance: "ibc-a", Authority: "owner", MinRequiredSignatures: 1}

	id := clientID("connection-ab", "A", client)
	require.Equal(
		t,
		"link-8b79fc938a76adf41b710b8f6a70dc97b29bef1e82ce7492840cdb8f4663512f",
		id,
	)
	require.Equal(t, id, clientID("connection-ab", "A", NewClient{
		IBCInstance: "different-instance", Authority: "different-owner", MinRequiredSignatures: 2,
	}))
	require.NotEqual(t, id, clientID("connection-ab", "B", client))
	require.NotEqual(t, id, clientID("other", "A", client))
	require.Equal(t, "existing", clientID("connection-ab", "A", ExistingClient{ID: "existing"}))
}

func TestFailedAttestorStartRetainsPartialCleanup(t *testing.T) {
	spec := mixedProtocolSpec()
	instances := map[IBCInstanceID]*IBCInstance{
		"ibc-a": {id: "ibc-a"},
		"ibc-b": {id: "ibc-b"},
	}
	connections := map[ConnectionID]*Connection{
		"connection-ab": resolvedMixedConnection("connection-ab", instances, "signer"),
	}
	effects := &effectJournal{}
	var releases atomic.Int32
	startErr := errors.New("readiness failed")

	_, err := acquireAttestors(
		t.Context(),
		spec,
		connections,
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
				description: "stop partial Attestor",
				release: func(context.Context) error {
					releases.Add(1)
					return nil
				},
			}, startErr
		}},
		effects,
	)
	require.ErrorIs(t, err, startErr)
	require.Empty(t, effects.cleanup(t.Context()))
	require.EqualValues(t, 1, releases.Load())
}

func mixedProtocolSpec() Spec {
	return Spec{
		Chains: []ChainSpec{
			ManagedAnvil{ID: "chain-a", EVMChainID: 31337},
			AttachedEVM{ID: "chain-b", EVMChainID: 31338, Endpoint: "chain-b-rpc", Timing: testTiming()},
		},
		IBCInstances: []IBCInstanceSpec{
			NewIBCInstance{ID: "ibc-a", Chain: "chain-a", Authority: "instance-owner"},
			ExistingIBCInstance{ID: "ibc-b", Chain: "chain-b", Locator: "0x1000000000000000000000000000000000000001"},
		},
		Connections: []ConnectionSpec{{
			ID: "connection-ab",
			A: NewClient{
				IBCInstance: "ibc-a", Authority: "client-owner", MinRequiredSignatures: 1,
				Attestors: []AttestorSpec{{ID: "attestor-a", Authority: "attestor-signer"}},
			},
			B: ExistingClient{IBCInstance: "ibc-b", ID: "existing-client-b"},
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
		label:       clientLabel(id, "A"),
		instance:    instances["ibc-a"],
		id:          "client-a-onchain",
		lightClient: "0x2000000000000000000000000000000000000002", counterpartyID: "existing-client-b",
		attestors: []EVMAddress{attestor}, minRequiredSignatures: 1,
	}
	b := &IBCClient{
		label:       clientLabel(id, "B"),
		instance:    instances["ibc-b"],
		id:          "existing-client-b",
		lightClient: "0x3000000000000000000000000000000000000003", counterpartyID: "client-a-onchain",
		minRequiredSignatures: 1,
	}
	return &Connection{id: id, a: a, b: b}
}
