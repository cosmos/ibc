// SPDX-License-Identifier: Apache-2.0

package e2etest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/harness/clientkind"
	"github.com/cosmos/ibc/e2e/internal/harness/ibccli"
)

func TestMergeRelayerConnection(t *testing.T) {
	for _, tc := range []struct {
		name  string
		autoA bool
		autoB bool
	}{
		{name: "manual"},
		{name: "forward only", autoA: true},
		{name: "reverse only", autoB: true},
		{name: "both", autoA: true, autoB: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, reversed := range []bool{false, true} {
				a := ibccli.RelayerClientEnd{
					ChainID: "1", ClientID: "client-0", ClientType: clientkind.BesuQBFT,
				}
				b := ibccli.RelayerClientEnd{
					ChainID: "2", ClientID: "client-0", ClientType: clientkind.Attestation,
				}
				forward := ibccli.RelayerConnection{A: a, B: b}
				forward.A.AutoRelay = tc.autoA
				reverse := ibccli.RelayerConnection{A: b, B: a}
				reverse.A.AutoRelay = tc.autoB
				routes := []ibccli.RelayerConnection{forward, reverse}
				if reversed {
					routes[0], routes[1] = routes[1], routes[0]
				}

				var cfg ibccli.RelayerConfig
				indices := map[[4]string]int{}
				for _, route := range routes {
					mergeRelayerConnection(&cfg, indices, route)
				}
				// Repeated manual routes must not disable an enabled endpoint.
				mergeRelayerConnection(&cfg, indices, ibccli.RelayerConnection{A: b, B: a})
				a.AutoRelay = tc.autoA
				b.AutoRelay = tc.autoB
				require.Equal(t, []ibccli.RelayerConnection{{A: a, B: b}}, cfg.Connections)
			}
		})
	}
}

func TestMergeRelayerConnectionKeysBothEnds(t *testing.T) {
	a := ibccli.RelayerClientEnd{ChainID: "1", ClientID: "a", ClientType: clientkind.BesuQBFT}
	b := ibccli.RelayerClientEnd{ChainID: "1", ClientID: "b", ClientType: clientkind.Attestation}
	c := ibccli.RelayerClientEnd{ChainID: "2", ClientID: "c", ClientType: clientkind.Attestation}
	var cfg ibccli.RelayerConfig
	indices := map[[4]string]int{}
	mergeRelayerConnection(&cfg, indices, ibccli.RelayerConnection{A: b, B: a})
	mergeRelayerConnection(&cfg, indices, ibccli.RelayerConnection{A: a, B: c})
	require.Equal(t, []ibccli.RelayerConnection{{A: a, B: b}, {A: a, B: c}}, cfg.Connections)
}
