package environment

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	chainimpl "github.com/cosmos/ibc/e2e/internal/harness/chain"
	chainevm "github.com/cosmos/ibc/e2e/internal/harness/chain/evm"
)

func TestStartValidatesRuntimeBeforeAcquisition(t *testing.T) {
	called := false
	spec := Spec{Chains: []ChainSpec{AttachedEVM{
		ID: "attached", EVMChainID: 31338, Endpoint: "missing", Timing: testTiming(),
	}}}
	_, err := start(t.Context(), spec, Runtime{}, drivers{
		acquireChain: func(context.Context, ChainSpec, Runtime, workspace) (chainAcquisition, error) {
			called = true
			return chainAcquisition{}, nil
		},
	})
	require.ErrorContains(t, err, `no runtime endpoint binding for "missing"`)
	require.False(t, called)
}

func TestStartRejectsInvalidExistingInstanceLocatorBeforeAcquisition(t *testing.T) {
	called := false
	spec := Spec{
		Chains: []ChainSpec{AttachedEVM{
			ID: "anvil", EVMChainID: 31337, Endpoint: "anvil-rpc", Timing: testTiming(),
		}},
		IBCInstances: []IBCInstanceSpec{
			ExistingIBCInstance{ID: "ibc", Chain: "anvil", Locator: "not-an-address"},
		},
	}
	_, err := start(t.Context(), spec, Runtime{Endpoints: map[EndpointBindingID]EndpointBinding{
		"anvil-rpc": {RPCURL: "http://127.0.0.1:8545"},
	}}, drivers{
		acquireChain: func(context.Context, ChainSpec, Runtime, workspace) (chainAcquisition, error) {
			called = true
			return chainAcquisition{}, nil
		},
	})
	require.ErrorContains(t, err, `IBC Instance locator "not-an-address" is not an EVM address`)
	require.False(t, called)
}

func TestStartRejectsAttestorSignerReuseAcrossClientsBeforeAcquisition(t *testing.T) {
	called := false
	spec := Spec{
		Chains: []ChainSpec{
			AttachedEVM{ID: "chain-a", EVMChainID: 31337, Endpoint: "chain-a-rpc", Timing: testTiming()},
			AttachedEVM{ID: "chain-b", EVMChainID: 31338, Endpoint: "chain-b-rpc", Timing: testTiming()},
		},
		IBCInstances: []IBCInstanceSpec{
			ExistingIBCInstance{ID: "ibc-a", Chain: "chain-a", Locator: "0x1000000000000000000000000000000000000001"},
			ExistingIBCInstance{ID: "ibc-b", Chain: "chain-b", Locator: "0x2000000000000000000000000000000000000002"},
		},
		Connections: []ConnectionSpec{{
			ID: "connection-ab",
			A:  ExistingClient{ID: "client-a", IBCInstance: "ibc-a", Locator: "existing-a"},
			B:  ExistingClient{ID: "client-b", IBCInstance: "ibc-b", Locator: "existing-b"},
		}},
		Attestors: []AttestorSpec{
			{ID: "attestor-a", Client: "client-a", Authority: "signer-a"},
			{ID: "attestor-b", Client: "client-b", Authority: "signer-b"},
		},
	}
	runtime := Runtime{
		Endpoints: map[EndpointBindingID]EndpointBinding{
			"chain-a-rpc": {RPCURL: "http://127.0.0.1:8545"},
			"chain-b-rpc": {RPCURL: "http://127.0.0.1:8546"},
		},
		Authorities: map[AuthorityID]EVMAuthority{
			"signer-a": {PrivateKeyHex: testPrimaryPrivateKeyHex},
			"signer-b": {PrivateKeyHex: testPrimaryPrivateKeyHex},
		},
	}

	_, err := start(t.Context(), spec, runtime, drivers{
		acquireChain: func(context.Context, ChainSpec, Runtime, workspace) (chainAcquisition, error) {
			called = true
			return chainAcquisition{}, nil
		},
	})
	require.ErrorContains(
		t,
		err,
		`Attestors "attestor-a" for IBC Client "client-a" and "attestor-b" for IBC Client "client-b" resolve to the same signer address`,
	)
	require.False(t, called)
}

func TestStartRejectsUnauthorizedNewClientBeforeAcquisition(t *testing.T) {
	called := false
	spec := mixedProtocolSpec()
	runtime := mixedProtocolRuntime()
	runtime.Authorities["client-owner"] = EVMAuthority{PrivateKeyHex: testSecondaryPrivateKeyHex}

	_, err := start(t.Context(), spec, runtime, drivers{
		acquireChain: func(context.Context, ChainSpec, Runtime, workspace) (chainAcquisition, error) {
			called = true
			return chainAcquisition{}, nil
		},
	})
	require.ErrorContains(
		t,
		err,
		`new IBC Client "client-a" authority must resolve to the new IBC Instance "ibc-a" admin address`,
	)
	require.False(t, called)
}

func TestValidateChecksSpecAndRuntime(t *testing.T) {
	require.NoError(t, Validate(mixedProtocolSpec(), mixedProtocolRuntime()))

	badSpec := mixedProtocolSpec()
	badSpec.Chains = append(badSpec.Chains, ManagedAnvil{ID: "chain-a", EVMChainID: 31339})
	require.ErrorContains(t, Validate(badSpec, mixedProtocolRuntime()), `duplicate Chain id "chain-a"`)

	badRuntime := mixedProtocolRuntime()
	delete(badRuntime.Endpoints, "chain-b-rpc")
	require.ErrorContains(t, Validate(mixedProtocolSpec(), badRuntime), `no runtime endpoint binding for "chain-b-rpc"`)
}

func TestProductionPrerequisitesRequireExecutableAttestorBinary(t *testing.T) {
	spec := Spec{Attestors: []AttestorSpec{{ID: "attestor-a"}}}
	path := filepath.Join(t.TempDir(), "ibc")
	t.Setenv("IBC_BIN", path)

	err := validateProductionPrerequisites(spec, Runtime{})
	require.ErrorContains(t, err, "no such file or directory")

	require.NoError(t, os.WriteFile(path, []byte("binary placeholder"), 0o600))
	err = validateProductionPrerequisites(spec, Runtime{})
	require.ErrorContains(t, err, "not an executable file")

	require.NoError(t, os.Chmod(path, 0o700))
	require.NoError(t, validateProductionPrerequisites(spec, Runtime{}))
}

func TestStartFailureIsAtomicAndCleansAcquiredEffects(t *testing.T) {
	primary := errors.New("second Chain failed with endpoint detail")
	firstAcquired := make(chan struct{})
	var releases atomic.Int32

	spec := Spec{Chains: []ChainSpec{
		ManagedAnvil{ID: "first", EVMChainID: 31337},
		ManagedAnvil{ID: "second", EVMChainID: 31338},
	}}
	env, err := start(t.Context(), spec, Runtime{}, drivers{
		acquireChain: func(_ context.Context, declaration ChainSpec, _ Runtime, _ workspace) (chainAcquisition, error) {
			switch declaration.chainID() {
			case "first":
				close(firstAcquired)
				return fakeAcquisition(
					"first",
					func(context.Context) error {
						releases.Add(1)
						return nil
					},
				), nil
			case "second":
				<-firstAcquired
				return chainAcquisition{}, primary
			default:
				panic("unexpected Chain")
			}
		},
	})
	require.Nil(t, env)
	var startErr *StartError
	require.ErrorAs(t, err, &startErr)
	require.ErrorIs(t, err, primary)
	require.Contains(t, err.Error(), "endpoint detail")
	require.EqualValues(t, 1, releases.Load())
}

func TestStartFailureReportsCleanupFailure(t *testing.T) {
	primary := errors.New("acquire failed")
	releaseErr := errors.New("release failed with details")
	firstAcquired := make(chan struct{})
	var privateDir, diagnosticsDir string
	spec := Spec{Chains: []ChainSpec{
		ManagedAnvil{ID: "first", EVMChainID: 31337},
		ManagedAnvil{ID: "second", EVMChainID: 31338},
	}}

	_, err := start(t.Context(), spec, Runtime{}, drivers{
		acquireChain: func(_ context.Context, declaration ChainSpec, _ Runtime, ws workspace) (chainAcquisition, error) {
			if declaration.chainID() == "first" {
				privateDir = ws.privateDir
				diagnosticsDir = ws.diagnosticsDir
				close(firstAcquired)

				attestorDir := filepath.Join(privateDir, "attestor", "keys")
				if mkdirErr := os.MkdirAll(attestorDir, 0o700); mkdirErr != nil {
					return chainAcquisition{}, mkdirErr
				}
				if writeErr := os.WriteFile(
					filepath.Join(attestorDir, "attestor.json"),
					[]byte(`{"privateKeyBase64":"must-not-be-diagnostic"}`),
					0o600,
				); writeErr != nil {
					return chainAcquisition{}, writeErr
				}
				if writeErr := os.WriteFile(
					filepath.Join(privateDir, "attestor", "ibc.yml"),
					[]byte("signer: private\n"),
					0o600,
				); writeErr != nil {
					return chainAcquisition{}, writeErr
				}
				if writeErr := os.WriteFile(
					filepath.Join(diagnosticsDir, "safe.log"),
					[]byte("safe diagnostic\n"),
					0o600,
				); writeErr != nil {
					return chainAcquisition{}, writeErr
				}
				return fakeAcquisition(
					"first",
					func(context.Context) error {
						return releaseErr
					},
				), nil
			}
			<-firstAcquired
			return chainAcquisition{}, primary
		},
	})
	var startErr *StartError
	require.ErrorAs(t, err, &startErr)
	require.ErrorIs(t, startErr.CleanupError(), releaseErr)
	require.Contains(t, err.Error(), "release failed with details")
	require.Contains(t, startErr.CleanupError().Error(), "release failed with details")
	require.Equal(t, diagnosticsDir, startErr.DiagnosticsDir())
	require.NotEqual(t, privateDir, startErr.DiagnosticsDir())
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(startErr.DiagnosticsDir())) })
	_, statErr := os.Stat(startErr.DiagnosticsDir())
	require.NoError(t, statErr, "cleanup failure retains the diagnostics workspace")
	_, statErr = os.Stat(privateDir)
	require.ErrorIs(t, statErr, os.ErrNotExist, "startup failure removes runtime-private files")
	entries, readErr := os.ReadDir(startErr.DiagnosticsDir())
	require.NoError(t, readErr)
	require.Len(t, entries, 1)
	require.Equal(t, "safe.log", entries[0].Name())
}

func TestEnvironmentCloseSeparatesBorrowedResourceFromOwnedHandle(t *testing.T) {
	var managedReleases atomic.Int32
	var attachedReleases atomic.Int32
	spec := Spec{Chains: []ChainSpec{
		ManagedAnvil{ID: "managed", EVMChainID: 31337},
		AttachedEVM{ID: "attached", EVMChainID: 31338, Endpoint: "attached-rpc", Timing: testTiming()},
	}}
	runtime := Runtime{Endpoints: map[EndpointBindingID]EndpointBinding{
		"attached-rpc": {RPCURL: "http://runtime-only.invalid"},
	}}

	env, err := start(t.Context(), spec, runtime, drivers{
		acquireChain: func(_ context.Context, declaration ChainSpec, _ Runtime, _ workspace) (chainAcquisition, error) {
			if declaration.chainID() == "managed" {
				return fakeAcquisition(
					"managed",
					func(context.Context) error {
						managedReleases.Add(1)
						return nil
					},
				), nil
			}
			return fakeAcquisition(
				"attached",
				func(context.Context) error {
					attachedReleases.Add(1)
					return nil
				},
			), nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, env)
	_, err = env.Chain("missing")
	require.ErrorContains(t, err, `no Chain "missing"`)

	require.NoError(t, env.Close(t.Context()))
	require.NoError(t, env.Close(t.Context()))
	require.EqualValues(t, 1, managedReleases.Load())
	require.EqualValues(t, 1, attachedReleases.Load())
}

func TestEnvironmentCloseRetriesOnlyFailedEffects(t *testing.T) {
	releaseErr := errors.New("temporary release failure")
	var calls atomic.Int32
	spec := Spec{Chains: []ChainSpec{ManagedAnvil{ID: "managed", EVMChainID: 31337}}}
	env, err := start(t.Context(), spec, Runtime{}, drivers{
		acquireChain: func(context.Context, ChainSpec, Runtime, workspace) (chainAcquisition, error) {
			acquisition := fakeAcquisition(
				"managed",
				func(context.Context) error {
					if calls.Add(1) == 1 {
						return releaseErr
					}
					return nil
				},
			)
			acquisition.chain.impl = fakeEVMRuntimeChain{fakeRuntimeChain{id: "managed"}}
			return acquisition, nil
		},
	})
	require.NoError(t, err)
	chain, err := env.Chain("managed")
	require.NoError(t, err)
	evmAccess, err := chain.EVM()
	require.NoError(t, err)

	require.ErrorIs(t, env.Close(t.Context()), releaseErr)
	require.ErrorIs(t, evmAccess.WaitNextPendingTx(t.Context()), ErrEnvironmentClosed)
	_, err = chain.Height(t.Context())
	require.ErrorIs(t, err, ErrEnvironmentClosed)
	require.NoError(t, env.Close(t.Context()))
	require.NoError(t, env.Close(t.Context()))
	require.EqualValues(t, 2, calls.Load(), "successful cleanup is not repeated")
}

func TestStartAcquiresIndependentChainsConcurrently(t *testing.T) {
	started := make(chan ChainID, 2)
	release := make(chan struct{})
	done := make(chan error, 1)
	spec := Spec{Chains: []ChainSpec{
		ManagedAnvil{ID: "a", EVMChainID: 31337},
		ManagedAnvil{ID: "b", EVMChainID: 31338},
	}}

	go func() {
		env, err := start(context.Background(), spec, Runtime{}, drivers{
			acquireChain: func(_ context.Context, declaration ChainSpec, _ Runtime, _ workspace) (chainAcquisition, error) {
				started <- declaration.chainID()
				<-release
				return fakeAcquisition(
					declaration.chainID(),
					func(context.Context) error {
						return nil
					},
				), nil
			},
		})
		if err == nil {
			err = env.Close(context.Background())
		}
		done <- err
	}()

	seen := map[ChainID]bool{}
	for range 2 {
		select {
		case id := <-started:
			seen[id] = true
		case <-time.After(time.Second):
			require.FailNow(t, "both independent Chain acquisitions did not start concurrently")
		}
	}
	close(release)
	require.NoError(t, <-done)
	require.Equal(t, map[ChainID]bool{"a": true, "b": true}, seen)
}

func TestStartSnapshotsRuntimeBindingsBeforeAcquisition(t *testing.T) {
	entered := make(chan struct{})
	proceed := make(chan struct{})
	seen := make(chan string, 1)
	runtime := Runtime{Endpoints: map[EndpointBindingID]EndpointBinding{
		"rpc": {RPCURL: "http://original.invalid"},
	}}
	spec := Spec{Chains: []ChainSpec{AttachedEVM{
		ID: "attached", EVMChainID: 31338, Endpoint: "rpc", Timing: testTiming(),
	}}}
	done := make(chan error, 1)
	go func() {
		env, err := start(context.Background(), spec, runtime, drivers{
			acquireChain: func(_ context.Context, _ ChainSpec, snapshot Runtime, _ workspace) (chainAcquisition, error) {
				close(entered)
				<-proceed
				seen <- snapshot.Endpoints["rpc"].RPCURL
				return fakeAcquisition(
					"attached",
					func(context.Context) error {
						return nil
					},
				), nil
			},
		})
		if err == nil {
			err = env.Close(context.Background())
		}
		done <- err
	}()

	<-entered
	runtime.Endpoints["rpc"] = EndpointBinding{RPCURL: "http://mutated.invalid"}
	close(proceed)
	require.Equal(t, "http://original.invalid", <-seen)
	require.NoError(t, <-done)
}

func fakeAcquisition(
	id ChainID,
	release func(context.Context) error,
) chainAcquisition {
	impl := fakeRuntimeChain{id: string(id)}
	return chainAcquisition{
		chain: &Chain{
			id:         id,
			evmChainID: 1,
			rpcURL:     "http://rpc.example.invalid",
			timing:     blockTiming(time.Second),
			impl:       impl,
		},
		description: "release Chain " + string(id),
		release:     release,
	}
}

type fakeRuntimeChain struct{ id string }

var _ chainimpl.Chain = fakeRuntimeChain{}

func (c fakeRuntimeChain) ID() string                           { return c.id }
func (fakeRuntimeChain) RPCURL() string                         { return "http://rpc.example.invalid" }
func (fakeRuntimeChain) Height(context.Context) (uint64, error) { return 1, nil }

type fakeEVMRuntimeChain struct{ fakeRuntimeChain }

func (fakeEVMRuntimeChain) WithEVMClient(use func(*chainevm.EVMClient) error) error {
	return use(nil)
}

func testTiming() Timing {
	return Timing{
		BlockInterval: time.Second, CompletionBudget: 20 * time.Second,
		PollInterval: 100 * time.Millisecond,
	}
}
