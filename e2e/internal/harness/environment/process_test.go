package environment

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/harness/ibclink"
)

func TestBindIBCLinkFollowsEnvironmentLifetime(t *testing.T) {
	env := newProcessBindingTestEnvironment(t)
	driver, err := ibclink.NewDriver(filepath.Join(t.TempDir(), "ibc-link.yaml"))
	require.NoError(t, err)
	require.NoError(t, env.BindIBCLink(driver))
	rpc, err := driver.ChainRPC("managed")
	require.NoError(t, err)
	require.Contains(t, rpc.URL, "${IBC_LINK_CHAIN_RPC_")

	require.NoError(t, env.Close(t.Context()))
	_, err = driver.ChainRPC("managed")
	require.NoError(t, err, "configuration references remain usable")
	require.ErrorIs(t, env.BindIBCLink(driver), ErrEnvironmentClosed)
}

func TestResolvedChainErrorsPassThrough(t *testing.T) {
	const endpoint = "https://api-user:secret-password@rpc.example.invalid/?token=query-secret"
	cause := errors.New("transport failed")
	chain := &Chain{
		id:     "failing",
		rpcURL: endpoint,
		impl: formattingChain{
			endpoint: endpoint,
			err: fmt.Errorf(
				"GET https://api-user:***@rpc.example.invalid/?token=query-secret: %w",
				cause,
			),
		},
	}

	_, err := chain.Height(t.Context())
	require.ErrorIs(t, err, cause)
	require.Contains(t, err.Error(), "api-user")
	require.Contains(t, err.Error(), "query-secret")
}

func TestResolvedCapabilityErrorsPassThrough(t *testing.T) {
	cause := errors.New("funding failed")
	controller := &failingEOAFunder{err: fmt.Errorf(
		"POST https://api-user:***@rpc.example.invalid/?token=query-secret: %w",
		cause,
	)}
	chain := &Chain{id: "failing"}
	funding := &Funding{
		controller: controller,
		chain:      chain,
	}

	err := funding.EnsureEOABalance(t.Context(), common.Address{}, big.NewInt(1))
	require.ErrorIs(t, err, cause)
	require.Contains(t, err.Error(), "api-user")
	require.Contains(t, err.Error(), "query-secret")
}

func TestBoundOneShotProcessLinearizesWithEnvironmentClose(t *testing.T) {
	started, release := processBindingMarkers(t)
	script := writeProcessBindingExecutable(t, `#!/bin/sh
set -eu
: > "$IBC_LINK_BINDING_TEST_STARTED"
while [ ! -f "$IBC_LINK_BINDING_TEST_RELEASE" ]; do sleep 0.01; done
printf '{"valid":true}\n'
`)
	t.Setenv("IBC_STUB_BIN", script)

	env := newProcessBindingTestEnvironment(t)
	driver, err := ibclink.NewDriver(filepath.Join(t.TempDir(), "ibc-link.yaml"))
	require.NoError(t, err)
	require.NoError(t, env.BindIBCLink(driver))

	commandDone := make(chan error, 1)
	go func() {
		_, validateErr := driver.ValidateConfig(t.Context(), false)
		commandDone <- validateErr
	}()
	waitForProcessBindingMarker(t, started)
	closeDone := closeEnvironmentAsync(env)
	requireCloseBlocked(t, closeDone)
	require.NoError(t, os.WriteFile(release, nil, 0o600))
	require.NoError(t, <-commandDone)
	require.NoError(t, <-closeDone)

	require.NoError(t, os.Remove(started))
	_, err = driver.ValidateConfig(t.Context(), false)
	require.ErrorIs(t, err, ErrEnvironmentClosed)
	_, statErr := os.Stat(started)
	require.ErrorIs(t, statErr, os.ErrNotExist, "closed Environment must prevent process launch")
}

func TestEnvironmentCloseHonorsDeadlineWhileBoundProcessRuns(t *testing.T) {
	started, release := processBindingMarkers(t)
	script := writeProcessBindingExecutable(t, `#!/bin/sh
set -eu
: > "$IBC_LINK_BINDING_TEST_STARTED"
while [ ! -f "$IBC_LINK_BINDING_TEST_RELEASE" ]; do sleep 0.01; done
printf '{"valid":true}\n'
`)
	t.Setenv("IBC_STUB_BIN", script)

	env := newProcessBindingTestEnvironment(t)
	driver, err := ibclink.NewDriver(filepath.Join(t.TempDir(), "ibc-link.yaml"))
	require.NoError(t, err)
	require.NoError(t, env.BindIBCLink(driver))

	commandDone := make(chan error, 1)
	go func() {
		_, validateErr := driver.ValidateConfig(t.Context(), false)
		commandDone <- validateErr
	}()
	waitForProcessBindingMarker(t, started)

	closeCtx, cancelClose := context.WithTimeout(context.Background(), 50*time.Millisecond)
	err = env.Close(closeCtx)
	cancelClose()
	require.ErrorIs(t, err, context.DeadlineExceeded)

	require.NoError(t, os.WriteFile(release, nil, 0o600))
	require.NoError(t, <-commandDone)
	require.NoError(t, env.Close(context.Background()))
}

func TestBoundRelayerStartLinearizesWithEnvironmentClose(t *testing.T) {
	started, release := processBindingMarkers(t)
	testBinary, err := os.Executable()
	require.NoError(t, err)
	t.Setenv("IBC_LINK_BINDING_TEST_BINARY", testBinary)
	script := writeProcessBindingExecutable(t, `#!/bin/sh
set -eu
IBC_LINK_BINDING_RELAYER_HELPER=1 exec "$IBC_LINK_BINDING_TEST_BINARY" \
  -test.run '^TestBoundRelayerHelperProcess$' -- "$@"
`)
	t.Setenv("IBC_STUB_BIN", script)

	env := newProcessBindingTestEnvironment(t)
	driver, err := ibclink.NewDriver(filepath.Join(t.TempDir(), "ibc-link.yaml"))
	require.NoError(t, err)
	require.NoError(t, env.BindIBCLink(driver))

	type startResult struct {
		relayer *ibclink.Relayer
		err     error
	}
	startDone := make(chan startResult, 1)
	go func() {
		relayer, startErr := driver.StartRelayer(t.Context())
		startDone <- startResult{relayer: relayer, err: startErr}
	}()
	waitForProcessBindingMarker(t, started)
	closeDone := closeEnvironmentAsync(env)
	requireCloseBlocked(t, closeDone)
	require.NoError(t, os.WriteFile(release, nil, 0o600))

	result := <-startDone
	require.NoError(t, result.err)
	require.NotNil(t, result.relayer)
	requireCloseBlocked(t, closeDone)
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
	require.NoError(t, result.relayer.Stop(stopCtx))
	stopCancel()
	require.NoError(t, <-closeDone)

	require.NoError(t, os.Remove(started))
	_, err = driver.StartRelayer(t.Context())
	require.ErrorIs(t, err, ErrEnvironmentClosed)
	_, statErr := os.Stat(started)
	require.ErrorIs(t, statErr, os.ErrNotExist, "closed Environment must prevent daemon launch")
}

func TestBoundProcessOutputPassesThrough(t *testing.T) {
	const endpoint = "https://api-user:password@rpc.example.invalid/?token=value"
	t.Run("one-shot stderr", func(t *testing.T) {
		script := writeProcessBindingExecutable(t, `#!/bin/sh
set -eu
printf '{"valid":false}\n'
printf '%s\n' '`+endpoint+`' >&2
exit 64
`)
		t.Setenv("IBC_STUB_BIN", script)
		env := newProcessBindingTestEnvironment(t)
		env.chains["managed"].rpcURL = endpoint
		t.Cleanup(func() { require.NoError(t, env.Close(context.Background())) })
		driver, err := ibclink.NewDriver(filepath.Join(t.TempDir(), "ibc-link.yaml"))
		require.NoError(t, err)
		require.NoError(t, env.BindIBCLink(driver))

		_, err = driver.ValidateConfig(t.Context(), false)
		require.Error(t, err)
		require.Contains(t, err.Error(), endpoint)
	})

	t.Run("daemon readiness", func(t *testing.T) {
		script := writeProcessBindingExecutable(t, `#!/bin/sh
set -eu
printf '%s\n' '`+endpoint+`'
`)
		t.Setenv("IBC_STUB_BIN", script)
		env := newProcessBindingTestEnvironment(t)
		env.chains["managed"].rpcURL = endpoint
		t.Cleanup(func() { require.NoError(t, env.Close(context.Background())) })
		driver, err := ibclink.NewDriver(filepath.Join(t.TempDir(), "ibc-link.yaml"))
		require.NoError(t, err)
		require.NoError(t, env.BindIBCLink(driver))

		_, err = driver.StartRelayer(t.Context())
		require.Error(t, err)
		require.Contains(t, err.Error(), endpoint)
	})
}

type failingEOAFunder struct {
	err error
}

func (f *failingEOAFunder) EnsureEOABalance(context.Context, common.Address, *big.Int) error {
	return f.err
}

type formattingChain struct {
	endpoint string
	err      error
}

func (formattingChain) ID() string                               { return "failing" }
func (c formattingChain) RPCURL() string                         { return c.endpoint }
func (c formattingChain) Height(context.Context) (uint64, error) { return 0, c.err }

func TestBoundRelayerHelperProcess(_ *testing.T) {
	if os.Getenv("IBC_LINK_BINDING_RELAYER_HELPER") != "1" {
		return
	}
	if err := runBoundRelayerHelper(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

func newProcessBindingTestEnvironment(t *testing.T) *Environment {
	t.Helper()
	env, err := start(t.Context(), Spec{Chains: []ChainSpec{
		ManagedAnvil{ID: "managed", EVMChainID: 31337},
	}}, Runtime{}, drivers{
		acquireChain: func(context.Context, ChainSpec, Runtime, workspace) (chainAcquisition, error) {
			return fakeAcquisition(
				"managed",
				func(context.Context) error { return nil },
			), nil
		},
	})
	require.NoError(t, err)
	return env
}

func processBindingMarkers(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	started := filepath.Join(dir, "started")
	release := filepath.Join(dir, "release")
	t.Setenv("IBC_LINK_BINDING_TEST_STARTED", started)
	t.Setenv("IBC_LINK_BINDING_TEST_RELEASE", release)
	return started, release
}

func writeProcessBindingExecutable(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ibc-binding-helper")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o700))
	return path
}

func waitForProcessBindingMarker(t *testing.T, path string) {
	t.Helper()
	require.Eventually(t, func() bool {
		_, err := os.Stat(path)
		return err == nil
	}, 10*time.Second, 10*time.Millisecond)
}

func closeEnvironmentAsync(env *Environment) <-chan error {
	done := make(chan error, 1)
	go func() { done <- env.Close(context.Background()) }()
	return done
}

func requireCloseBlocked(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		t.Fatalf("Environment.Close returned while a bound process was starting: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
}

func runBoundRelayerHelper() error {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	server := &http.Server{Handler: mux, ReadHeaderTimeout: time.Second}
	go func() { _ = server.Serve(listener) }()
	defer func() { _ = server.Close() }()

	if err := os.WriteFile(os.Getenv("IBC_LINK_BINDING_TEST_STARTED"), nil, 0o600); err != nil {
		return err
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(os.Getenv("IBC_LINK_BINDING_TEST_RELEASE")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for process binding release")
		}
		time.Sleep(10 * time.Millisecond)
	}
	fmt.Printf(
		"{\"event\":\"ready\",\"configLoaded\":true,\"dbReady\":true,"+
			"\"chainsConnected\":[\"managed\"],\"relayerSubscribed\":true,"+
			"\"status\":{\"http\":%q}}\n",
		listener.Addr().String(),
	)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)
	defer signal.Stop(signals)
	select {
	case <-signals:
		return nil
	case <-time.After(10 * time.Second):
		return fmt.Errorf("timed out waiting for stop signal")
	}
}
