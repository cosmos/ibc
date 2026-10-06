// SPDX-License-Identifier: Apache-2.0

package container

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"testing"

	"github.com/containerd/errdefs"
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

func TestIsPortConflict(t *testing.T) {
	hostBindMessage := "failed to set up container networking: " +
		"failed to bind host port for 127.0.0.1::172.17.0.3:8545/tcp: address already in use"
	tcpBindMessage := "failed to set up container networking: failed to listen on TCP socket: address already in use"

	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{name: "success"},
		{
			name: "host port binding without testcontainers wrapper",
			err:  errdefs.ErrInternal.WithMessage(hostBindMessage),
			want: true,
		},
		{
			name: "TCP socket binding with arbitrary wrapper",
			err:  fmt.Errorf("launch failed: %w", errdefs.ErrInternal.WithMessage(tcpBindMessage)),
			want: true,
		},
		{
			name: "readiness failure",
			err:  errors.New("started hook: wait until ready: address already in use"),
		},
		{
			name: "readiness failure quoting daemon text",
			err:  fmt.Errorf("wait until ready: %s", hostBindMessage),
		},
		{
			name: "other internal error with address in use",
			err:  errdefs.ErrInternal.WithMessage("exec failed: address already in use"),
		},
		{
			name: "other Docker startup error",
			err:  errdefs.ErrInternal.WithMessage("failed to set up container networking: permission denied"),
		},
		{
			name: "name conflict",
			err:  errors.New("create container: the container name is already in use"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, isPortConflict(tc.err))
		})
	}
}

func TestStartRetriesPortConflicts(t *testing.T) {
	portErr := errdefs.ErrInternal.WithMessage(
		"failed to set up container networking: failed to listen on TCP socket: address already in use",
	)
	otherErr := errors.New("container start: permission denied")
	cleanupErr := errors.New("Docker removal failed")

	for _, tc := range []struct {
		name             string
		failures         []error
		cleanupErr       error
		cancel           bool
		missingContainer bool
		wantErr          error
		wantEvents       []string
	}{
		{
			name:       "success",
			wantEvents: []string{"start"},
		},
		{
			name:       "recreates after collision",
			failures:   []error{portErr},
			wantEvents: []string{"start", "terminate", "start"},
		},
		{
			name:       "bounded retries",
			failures:   []error{portErr, portErr, portErr},
			wantErr:    portErr,
			wantEvents: []string{"start", "terminate", "start", "terminate", "start", "terminate"},
		},
		{
			name:       "unrelated failure is not retried",
			failures:   []error{otherErr},
			wantErr:    otherErr,
			wantEvents: []string{"start", "terminate"},
		},
		{
			name:             "failure before creation",
			failures:         []error{otherErr},
			missingContainer: true,
			wantErr:          otherErr,
			wantEvents:       []string{"start"},
		},
		{
			name:       "cleanup failure prevents name reuse",
			failures:   []error{portErr},
			cleanupErr: cleanupErr,
			wantErr:    portErr,
			wantEvents: []string{"start", "terminate"},
		},
		{
			name:       "cancellation still cleans up",
			failures:   []error{portErr},
			cancel:     true,
			wantErr:    context.Canceled,
			wantEvents: []string{"start", "terminate"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			request := testcontainers.ContainerRequest{Name: "fixed-chain-name"}
			var events []string
			var created []*startTestContainer
			attempt := 0
			got, err := start(ctx, request, func(
				_ context.Context, req testcontainers.GenericContainerRequest,
			) (testcontainers.Container, error) {
				require.True(t, req.Started)
				require.Equal(t, request.Name, req.Name)
				events = append(events, "start")
				c := &startTestContainer{terminate: func(cleanupCtx context.Context) error {
					require.NoError(t, cleanupCtx.Err())
					_, hasDeadline := cleanupCtx.Deadline()
					require.True(t, hasDeadline)
					events = append(events, "terminate")
					return tc.cleanupErr
				}}
				created = append(created, c)
				if tc.cancel {
					cancel()
				}
				if attempt < len(tc.failures) {
					failure := tc.failures[attempt]
					attempt++
					if tc.missingContainer {
						return nil, failure
					}
					return c, failure
				}
				return c, nil
			})
			if tc.wantErr == nil {
				require.NoError(t, err)
				require.Same(t, created[len(created)-1], got)
			} else {
				require.ErrorIs(t, err, tc.wantErr)
				require.Nil(t, got)
			}
			if tc.cleanupErr != nil {
				require.ErrorIs(t, err, tc.cleanupErr)
			}
			require.Equal(t, tc.wantEvents, events)
		})
	}
}

type startTestContainer struct {
	testcontainers.Container
	terminate func(context.Context) error
}

func (c *startTestContainer) Terminate(ctx context.Context, _ ...testcontainers.TerminateOption) error {
	return c.terminate(ctx)
}
