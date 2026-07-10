package diag

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/harness/fixturekeys"
	"github.com/cosmos/ibc/link/harness/ibclink/wire"
)

// TestBundleRendersAllSections pins the full failure bundle: every section renders. Pure-Go so the
// every-PR lane guards the rendering without the e2e tag.
func TestBundleRendersAllSections(t *testing.T) {
	b := NewBundle()

	b.SetVersion("anvil", "anvil Version: 1.7.1")
	b.SetVersion("go-ethereum", "v1.16.9")
	b.SetVersion("ibc-link", "/repo/bin/ibc")
	b.SetTopology("name=evm-evm-anvil db=sqlite\nroute route-a-to-b: chain-a -> chain-b")
	b.SetConfig("relayer:\n  routes:\n    - id: route-a-to-b\n")
	b.SetDeployment(&wire.Deployment{
		Chains: map[string]wire.ChainDeployment{
			"chain-a": {Fixtures: map[string]string{
				fixturekeys.MockIFT: "0xIFT",
				fixturekeys.MockGMP: "0xGMP",
				fixturekeys.Counter: "0xCNT",
			}, ClientID: "client-a-b"},
		},
		TxHashes: []string{"0xdeadbeef"},
	})
	b.SetHeight("chain-a", 7)
	b.AddChainLog("chain-a", "anvil log line")
	b.AddSUTLog("ibc-link-daemon-1", "relayer log line")
	b.AddStatus("ibc-link-daemon-1", `{"packets":[{"packetId":"p1","sequence":1,"state":"complete"}]}`)

	out := b.String()

	// Every section is present.
	for _, want := range []string{
		"versions:", "anvil Version: 1.7.1", "v1.16.9", "/repo/bin/ibc",
		"topology:", "name=evm-evm-anvil",
		"deployment:", "0xIFT", "route-a-to-b", "0xdeadbeef",
		"heights:", "chain-a: 7",
		"resolved config",
		"chain chain-a log", "anvil log line",
		"sut ibc-link-daemon-1 log", "relayer log line",
		"sut ibc-link-daemon-1 status", `"packetId":"p1"`,
	} {
		require.Containsf(t, out, want, "bundle must render %q", want)
	}
}
