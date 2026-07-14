package containerutil

import (
	"net/netip"
	"testing"

	"github.com/moby/moby/api/types/network"
	"github.com/stretchr/testify/require"

	containertypes "github.com/moby/moby/api/types/container"
)

func TestNamePrefixSanitizesDockerNames(t *testing.T) {
	require.Equal(t, "ibc-link-e2e-run-id-chain-a", NamePrefix("Run ID", "Chain/A"))
	require.Equal(t, "ibc-link-e2e-run-run", NamePrefix("!!!", ""))
}

func TestBindExposedPortsToLoopbackPreservesDynamicPorts(t *testing.T) {
	config := &containertypes.HostConfig{PortBindings: network.PortMap{}}
	BindPortsToLoopback(config, "8545/tcp")
	port := network.MustParsePort("8545/tcp")
	require.Equal(t, netip.MustParseAddr("127.0.0.1"), config.PortBindings[port][0].HostIP)
	require.Equal(t, "0", config.PortBindings[port][0].HostPort)
}
