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

	containertypes "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/log"
)

var nameRe = regexp.MustCompile(`[^a-z0-9_.-]+`)

const startAttempts = 3

// Start retries managed chain startup with a fresh container after each failed start.
// Requests must have no wait strategy or custom lifecycle hooks; callers check RPC readiness separately.
func Start(ctx context.Context, request testcontainers.ContainerRequest) (testcontainers.Container, error) {
	return start(ctx, request, testcontainers.GenericContainer)
}

func start(
	ctx context.Context,
	request testcontainers.ContainerRequest,
	create func(context.Context, testcontainers.GenericContainerRequest) (testcontainers.Container, error),
) (testcontainers.Container, error) {
	var lastErr error
	for attempt := 1; attempt <= startAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, errors.Join(lastErr, err)
		}
		if attempt > 1 {
			log.Printf("Retrying container %s (attempt %d/%d): %v", request.Name, attempt, startAttempts, lastErr)
		}
		c, err := create(ctx, testcontainers.GenericContainerRequest{ContainerRequest: request})
		if err != nil {
			cleanupErr := Terminate(c)
			return nil, errors.Join(lastErr, err, cleanupErr, ctx.Err())
		}
		lastErr = c.Start(ctx)
		if lastErr == nil {
			return c, nil
		}
		// Restarting after a failed port bind can succeed without networking; remove before recreating the same name.
		if cleanupErr := Terminate(c); cleanupErr != nil {
			return nil, errors.Join(lastErr, fmt.Errorf("remove failed container: %w", cleanupErr), ctx.Err())
		}
	}
	return nil, fmt.Errorf("start container after %d attempts: %w", startAttempts, errors.Join(lastErr, ctx.Err()))
}

// Terminate discards a container even when the startup context has been canceled.
func Terminate(c testcontainers.Container) error {
	if c == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return c.Terminate(ctx, testcontainers.StopTimeout(time.Second))
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
