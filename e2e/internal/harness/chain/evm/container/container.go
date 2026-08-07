// Package container contains metadata shared by managed EVM test containers.
package container

import (
	"net/netip"
	"regexp"
	"strings"

	containertypes "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
)

var nameRe = regexp.MustCompile(`[^a-z0-9_.-]+`)

func Labels(runID string) map[string]string {
	return map[string]string{
		"ibc-link-e2e": "true",
		"ibc-link-run": runID,
	}
}

func NamePrefix(runID, chainID string) string {
	return "ibc-link-e2e-" + safe(runID) + "-" + safe(chainID)
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

func safe(value string) string {
	value = nameRe.ReplaceAllString(strings.ToLower(value), "-")
	value = strings.Trim(value, "-_.")
	if value == "" {
		return "run"
	}
	return value
}
