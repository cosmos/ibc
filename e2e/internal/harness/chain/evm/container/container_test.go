// SPDX-License-Identifier: Apache-2.0

package container

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	containertypes "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
)

// Observed on GitHub runners when Docker hands out a host port another socket holds.
var (
	errBindHostPort = errors.New("start container: Error response from daemon: failed to set up container " +
		"networking: driver failed programming external connectivity on endpoint chain-a-anvil (eb37): " +
		"failed to bind host port for 127.0.0.1::172.17.0.7:8545/tcp: address already in use")
	errListenSocket = errors.New("start container: Error response from daemon: failed to set up container " +
		"networking: driver failed programming external connectivity on endpoint chain-a-generate (7f14): " +
		"failed to listen on TCP socket: address already in use")
)

type fakeContainer struct {
	testcontainers.Container
	terminated int
}

func (c *fakeContainer) Terminate(context.Context, ...testcontainers.TerminateOption) error {
	c.terminated++
	return nil
}

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

func TestIsHostPortConflict(t *testing.T) {
	require.True(t, isHostPortConflict(errBindHostPort))
	require.True(t, isHostPortConflict(errListenSocket))
	require.True(t, isHostPortConflict(errors.New("Bind for 127.0.0.1:8545 failed: port is already allocated")))
	require.False(t, isHostPortConflict(errors.New("No such image: hyperledger/besu:missing")))
}

func TestStartRecreatesContainerAfterHostPortConflict(t *testing.T) {
	failed := &fakeContainer{}
	started := &fakeContainer{}
	attempts := 0
	ctr, err := startWithRetry(context.Background(), func(context.Context) (testcontainers.Container, error) {
		attempts++
		if attempts == 1 {
			return failed, errBindHostPort
		}
		return started, nil
	})
	require.NoError(t, err)
	require.Same(t, started, ctr)
	require.Equal(t, 2, attempts)
	require.Equal(t, 1, failed.terminated)
	require.Zero(t, started.terminated)
}

func TestStartGivesUpAfterRepeatedHostPortConflicts(t *testing.T) {
	last := &fakeContainer{}
	attempts := 0
	ctr, err := startWithRetry(context.Background(), func(context.Context) (testcontainers.Container, error) {
		attempts++
		if attempts == startAttempts {
			return last, errListenSocket
		}
		return &fakeContainer{}, errListenSocket
	})
	require.ErrorIs(t, err, errListenSocket)
	require.ErrorContains(t, err, "host port conflict after 5 attempts")
	require.Equal(t, startAttempts, attempts)
	require.Same(t, last, ctr)
	require.Zero(t, last.terminated, "the caller cleans up the final container")
}

func TestStartDoesNotRetryOtherErrors(t *testing.T) {
	startErr := errors.New("No such image: hyperledger/besu:missing")
	attempts := 0
	_, err := startWithRetry(context.Background(), func(context.Context) (testcontainers.Container, error) {
		attempts++
		return nil, startErr
	})
	require.ErrorIs(t, err, startErr)
	require.Equal(t, 1, attempts)
}
