// SPDX-License-Identifier: Apache-2.0

// Package container contains metadata shared by managed EVM test containers.
package container

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"strings"

	containertypes "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/testcontainers/testcontainers-go"
)

const startAttempts = 5

var nameRe = regexp.MustCompile(`[^a-z0-9_.-]+`)

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
// while allowing Docker to choose race-free host ports.
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

// Start creates and starts a container, recreating it when Docker cannot bind a
// dynamically allocated host port. Docker draws those ports from the kernel's
// ephemeral range, where outbound sockets on a busy host may already hold them.
// The container is returned with any error so callers can clean it up.
func Start(ctx context.Context, request testcontainers.GenericContainerRequest) (testcontainers.Container, error) {
	request.Started = true
	return startWithRetry(ctx, func(ctx context.Context) (testcontainers.Container, error) {
		return testcontainers.GenericContainer(ctx, request)
	})
}

func startWithRetry(
	ctx context.Context,
	start func(context.Context) (testcontainers.Container, error),
) (testcontainers.Container, error) {
	for attempt := 1; ; attempt++ {
		ctr, err := start(ctx)
		if err == nil || !isHostPortConflict(err) || ctx.Err() != nil {
			return ctr, err
		}
		if attempt == startAttempts {
			return ctr, fmt.Errorf("host port conflict after %d attempts: %w", attempt, err)
		}
		if cleanupErr := testcontainers.TerminateContainer(ctr); cleanupErr != nil {
			return ctr, errors.Join(err, fmt.Errorf("remove container after host port conflict: %w", cleanupErr))
		}
	}
}

func isHostPortConflict(err error) bool {
	message := err.Error()
	return strings.Contains(message, "address already in use") ||
		strings.Contains(message, "port is already allocated")
}

func safe(value string) string {
	value = nameRe.ReplaceAllString(strings.ToLower(value), "-")
	value = strings.Trim(value, "-_.")
	if value == "" {
		return "run"
	}
	return value
}
