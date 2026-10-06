// SPDX-License-Identifier: Apache-2.0

// Package container contains helpers shared by managed EVM test containers.
package container

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	containertypes "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/testcontainers/testcontainers-go"
)

var nameRe = regexp.MustCompile(`[^a-z0-9_.-]+`)

const startAttempts = 3

// Start recreates containers whose Docker-assigned host ports collide with host sockets.
func Start(ctx context.Context, request testcontainers.ContainerRequest) (testcontainers.Container, error) {
	return start(ctx, request, testcontainers.GenericContainer)
}

func start(
	ctx context.Context,
	request testcontainers.ContainerRequest,
	run func(context.Context, testcontainers.GenericContainerRequest) (testcontainers.Container, error),
) (testcontainers.Container, error) {
	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		c, err := run(ctx, testcontainers.GenericContainerRequest{ContainerRequest: request, Started: true})
		if err == nil {
			return c, nil
		}
		if c != nil {
			// Cleanup must finish before reusing the fixed name, even if startup was canceled.
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			cleanupErr := c.Terminate(cleanupCtx, testcontainers.StopTimeout(time.Second))
			cancel()
			if cleanupErr != nil {
				return nil, errors.Join(err, fmt.Errorf("remove failed container: %w", cleanupErr))
			}
		}
		if attempt == startAttempts || !isPortConflict(err) {
			return nil, fmt.Errorf("container start attempt %d/%d: %w", attempt, startAttempts, err)
		}
	}
}

func isPortConflict(err error) bool {
	// Docker's HTTP 500 networking-setup response includes the kernel's EADDRINUSE text.
	return errdefs.IsInternal(err) && strings.Contains(err.Error(), "failed to set up container networking") &&
		strings.Contains(err.Error(), "address already in use")
}

func Labels(runID string) map[string]string {
	return map[string]string{
		"ibc-cli-e2e": "true",
		"ibc-cli-run": runID,
	}
}

func NamePrefix(runID, chainID string) string {
	return "ibc-cli-e2e-" + safe(runID) + "-" + safe(chainID)
}

// BindPortsToLoopback keeps development RPC endpoints off external interfaces
// while allowing Docker to choose host ports.
func BindPortsToLoopback(config *containertypes.HostConfig, ports ...string) {
	if config.PortBindings == nil {
		config.PortBindings = network.PortMap{}
	}
	for _, value := range ports {
		port := network.MustParsePort(value)
		bindings := config.PortBindings[port]
		if len(bindings) == 0 {
			bindings = []network.PortBinding{{HostPort: "0"}}
		}
		for i := range bindings {
			bindings[i].HostIP = netip.MustParseAddr("127.0.0.1")
		}
		config.PortBindings[port] = bindings
	}
}

func safe(value string) string {
	value = nameRe.ReplaceAllString(strings.ToLower(value), "-")
	value = strings.Trim(value, "-_.")
	if value == "" {
		return "run"
	}
	return value
}
