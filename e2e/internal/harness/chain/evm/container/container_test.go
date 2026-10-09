// SPDX-License-Identifier: Apache-2.0

package container

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"testing"

	containertypes "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
)

func TestNamePrefixSanitizesDockerNames(t *testing.T) {
	require.Equal(t, "ibc-cli-e2e-run-id-chain-a", NamePrefix("Run ID", "Chain/A"))
	require.Equal(t, "ibc-cli-e2e-run-run", NamePrefix("!!!", ""))
}

func TestBindExposedPortsToLoopbackPreservesDynamicPorts(t *testing.T) {
	config := &containertypes.HostConfig{PortBindings: network.PortMap{}}
	BindPortsToLoopback(config, "8545/tcp")
	port := network.MustParsePort("8545/tcp")
	require.Equal(t, netip.MustParseAddr("127.0.0.1"), config.PortBindings[port][0].HostIP)
	require.Equal(t, "0", config.PortBindings[port][0].HostPort)
}

func TestStart(t *testing.T) {
	startErr := errors.New("daemon start failed")
	lastErr := errors.New("last start failed")
	createErr := errors.New("reaper connection failed")
	cleanupErr := errors.New("container removal failed")

	for _, tc := range []struct {
		name          string
		startErrors   []error
		createErr     error
		createAttempt int
		noContainer   bool
		cleanupErr    error
		cancelBefore  bool
		cancelOnStart bool
		wantErrors    []error
		wantEvents    string
	}{
		{
			name: "success", wantEvents: "create start",
		},
		{
			name: "recreates after failed start", startErrors: []error{startErr},
			wantEvents: "create start terminate create start",
		},
		{
			name: "exhaustion", startErrors: []error{startErr, startErr, lastErr},
			wantErrors: []error{lastErr},
			wantEvents: "create start terminate create start terminate create start terminate",
		},
		{
			name: "creation failure without container", createErr: createErr, noContainer: true,
			wantErrors: []error{createErr}, wantEvents: "create",
		},
		{
			name: "creation failure with container", createErr: createErr,
			wantErrors: []error{createErr}, wantEvents: "create terminate",
		},
		{
			name: "creation failure after failed start", startErrors: []error{startErr},
			createErr: createErr, createAttempt: 1, noContainer: true,
			wantErrors: []error{startErr, createErr}, wantEvents: "create start terminate create",
		},
		{
			name: "cleanup failure aborts", startErrors: []error{startErr}, cleanupErr: cleanupErr,
			wantErrors: []error{startErr, cleanupErr}, wantEvents: "create start terminate",
		},
		{
			name: "cancellation before creation", cancelBefore: true,
			wantErrors: []error{context.Canceled},
		},
		{
			name: "cancellation preserves startup failure", startErrors: []error{startErr}, cancelOnStart: true,
			wantErrors: []error{startErr, context.Canceled}, wantEvents: "create start terminate",
		},
		{
			name: "cancellation and cleanup failure", startErrors: []error{startErr},
			cancelOnStart: true, cleanupErr: cleanupErr,
			wantErrors: []error{startErr, cleanupErr, context.Canceled}, wantEvents: "create start terminate",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tc.cancelBefore {
				cancel()
			}
			var events []string
			var latest *startTestContainer
			attempt := 0
			got, err := start(ctx, testcontainers.ContainerRequest{Name: "fixed-chain-name"}, func(
				_ context.Context, req testcontainers.GenericContainerRequest,
			) (testcontainers.Container, error) {
				require.False(t, req.Started, "creation and start must be separate")
				require.Equal(t, "fixed-chain-name", req.Name)
				events = append(events, "create")
				var creationErr error
				if attempt == tc.createAttempt {
					creationErr = tc.createErr
				}
				if tc.noContainer && creationErr != nil {
					return nil, creationErr
				}
				latest = &startTestContainer{
					start: func(context.Context) error {
						events = append(events, "start")
						if tc.cancelOnStart {
							cancel()
						}
						index := attempt
						attempt++
						if index < len(tc.startErrors) {
							return tc.startErrors[index]
						}
						return nil
					},
					terminate: func(cleanupCtx context.Context) error {
						require.NoError(t, cleanupCtx.Err())
						_, hasDeadline := cleanupCtx.Deadline()
						require.True(t, hasDeadline)
						events = append(events, "terminate")
						return tc.cleanupErr
					},
				}
				return latest, creationErr
			})
			if len(tc.wantErrors) == 0 {
				require.NoError(t, err)
				require.Same(t, latest, got)
			} else {
				for _, want := range tc.wantErrors {
					require.ErrorIs(t, err, want)
				}
				require.Nil(t, got)
			}
			require.Equal(t, tc.wantEvents, strings.Join(events, " "))
		})
	}
}

type startTestContainer struct {
	testcontainers.Container
	start     func(context.Context) error
	terminate func(context.Context) error
}

func (c *startTestContainer) Start(ctx context.Context) error {
	return c.start(ctx)
}

func (c *startTestContainer) Terminate(ctx context.Context, _ ...testcontainers.TerminateOption) error {
	return c.terminate(ctx)
}
