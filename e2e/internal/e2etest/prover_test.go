// SPDX-License-Identifier: Apache-2.0

package e2etest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/harness/ibclink"
)

// The prover and relayer configs are separate files, so only the identity
// comparison stops them drifting. Each identity must therefore change when the
// thing it names changes.
func TestConfigIdentitiesDetectDrift(t *testing.T) {
	t.Parallel()

	base := ibclink.RelayerConfig{
		Chains: []ibclink.RelayerChain{{ChainID: "1", RPC: "http://a", ICS26Router: "0xrouter"}},
		Attestors: []ibclink.RelayerAttestor{
			{Name: "att-a", Type: ibclink.RelayerAttestorRemote, GRPC: "127.0.0.1:1"},
		},
		Connections: []ibclink.RelayerConnection{
			{ChainA: "1", ClientA: "client-a", ChainB: "2", ClientB: "client-b"},
		},
	}

	for _, tt := range []struct {
		name     string
		mutate   func(*ibclink.RelayerConfig)
		identity func(ibclink.RelayerConfig) []string
	}{
		{"chain id", func(c *ibclink.RelayerConfig) { c.Chains[0].ChainID = "99" }, chainIdentities},
		{"router", func(c *ibclink.RelayerConfig) { c.Chains[0].ICS26Router = "0xother" }, chainIdentities},
		{"chain added", func(c *ibclink.RelayerConfig) {
			c.Chains = append(c.Chains, ibclink.RelayerChain{ChainID: "2"})
		}, chainIdentities},
		{"attestor name", func(c *ibclink.RelayerConfig) { c.Attestors[0].Name = "att-b" }, attestorIdentities},
		{"attestor endpoint", func(c *ibclink.RelayerConfig) { c.Attestors[0].GRPC = "127.0.0.1:2" }, attestorIdentities},
		{"client id", func(c *ibclink.RelayerConfig) { c.Connections[0].ClientA = "other" }, clientIdentities},
		{"client chain", func(c *ibclink.RelayerConfig) { c.Connections[0].ChainB = "3" }, clientIdentities},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			drifted := cloneConfig(base)
			tt.mutate(&drifted)

			require.NotElementsMatch(t, tt.identity(base), tt.identity(drifted),
				"drift in %s must change the identity", tt.name)
		})
	}

	// A config that did not drift must still match, or the comparison would
	// fail every run and be turned off.
	require.ElementsMatch(t, chainIdentities(base), chainIdentities(cloneConfig(base)))
	require.ElementsMatch(t, attestorIdentities(base), attestorIdentities(cloneConfig(base)))
	require.ElementsMatch(t, clientIdentities(base), clientIdentities(cloneConfig(base)))
}

func cloneConfig(cfg ibclink.RelayerConfig) ibclink.RelayerConfig {
	out := cfg
	out.Chains = append([]ibclink.RelayerChain(nil), cfg.Chains...)
	out.Attestors = append([]ibclink.RelayerAttestor(nil), cfg.Attestors...)
	out.Connections = append([]ibclink.RelayerConnection(nil), cfg.Connections...)

	return out
}
