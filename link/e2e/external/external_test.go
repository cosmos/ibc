// Package external_test proves the harness can drive a topology whose destination chain runs OUT OF BAND:
// the harness connects to it as an external chain (no launch, no teardown, no block control) while still
// relaying a real IFT through it. It is the Phase 1 proof that provisioning ("how the harness brings a
// chain up") is separable from the ibc link config ("what the relayer is told").
package external_test

import (
	"math/big"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/e2e/e2etest"
	"github.com/cosmos/ibc/link/harness"
	"github.com/cosmos/ibc/link/harness/chain/evm/anvil"
	"github.com/cosmos/ibc/link/harness/ibclink/wire"
	"github.com/cosmos/ibc/link/harness/testkeys"
	"github.com/cosmos/ibc/link/harness/topology"
)

// Ad-hoc topologies pick their own chain ids (see the reserved-ranges note in harness/topology):
// chain-a keeps anvil's default id, and the out-of-band anvil takes a distinct one so the config's
// live chain-id check on the external chain is exercised against a genuinely different node.
const (
	managedChainID  = 31337
	externalChainID = 31347
)

func TestExternalChain_HarnessConnectsButDoesNotOwn(t *testing.T) {
	// External chains dial an anvil the harness starts here; keep it in the default (anvil) lane (dedup —
	// no block-control capability is used).
	e2etest.RequireAnvilLane(t)
	ctx := t.Context()

	// Launch an anvil OUT OF BAND: the harness must connect to it as an external chain, not own it. The
	// dev faucet works against it only because it is an anvil with the default prefunded genesis — the
	// named POC funding gap documented on external.Chain.
	oob, err := anvil.Start(ctx, anvil.Spec{
		ID:      "chain-b-external",
		ChainID: externalChainID,
		LogPath: filepath.Join(t.TempDir(), "external-anvil.log"),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = oob.Stop() })

	startHeight, err := oob.Height(ctx)
	require.NoError(t, err)

	// chain-a is a managed anvil the harness launches; chain-b is external, pointing at the out-of-band
	// node above. Only Provision differs — the ibc link config still describes two ordinary EVM chains.
	topo := topology.Topology{
		Name: "evm-evm-external-proof",
		Chains: []topology.ChainSpec{
			{
				Chain: wire.Chain{
					ID:           topology.ChainA,
					Type:         wire.ChainTypeEVM,
					Provider:     wire.ProviderAnvil,
					ChainID:      managedChainID,
					EVMSignerKey: testkeys.RelayerPrivateKeyHex,
				},
				Provision: topology.Provision{Mode: topology.ProvisionManaged, Launcher: topology.LauncherAnvil},
			},
			{
				Chain: wire.Chain{
					ID:           topology.ChainB,
					Type:         wire.ChainTypeEVM,
					Provider:     wire.ProviderAnvil,
					ChainID:      externalChainID,
					EVMSignerKey: testkeys.RelayerPrivateKeyHex,
				},
				Provision: topology.Provision{Mode: topology.ProvisionExternal, RPCURL: oob.RPCURL()},
			},
		},
		Config: wire.ConfigYAML{
			Relayer: wire.Relayer{
				Routes: []wire.Route{
					{
						ID:          topology.RouteAtoB,
						Source:      topology.ChainA,
						Destination: topology.ChainB,
						Type:        wire.RouteEVMToEVMAttested,
					},
					{
						ID:          topology.RouteBtoA,
						Source:      topology.ChainB,
						Destination: topology.ChainA,
						Type:        wire.RouteEVMToEVMAttested,
					},
				},
			},
		},
	}

	// Do the harness work in a subtest so its teardown (registered via e2etest.Start's t.Cleanup) runs
	// when the subtest returns — BEFORE the liveness assertion below. That ordering is what makes "harness
	// teardown never stopped the external node" a real check rather than a vacuous one.
	t.Run("harness", func(t *testing.T) {
		run := e2etest.Start(t, topo)
		rctx := t.Context()

		// A real IFT relays source(managed) -> destination(external): mint lands on the out-of-band node.
		out, err := run.IFT(rctx, harness.IFT{Route: topology.RouteAtoB, Amount: big.NewInt(1_500_000)})
		require.NoError(t, err)
		require.NoError(t, out.VerifyComplete(rctx))

		// The external chain is connected, not owned: block-control capabilities are deliberately absent,
		// so a PauseMining request fails the standard capability negotiation.
		err = run.Chain(topology.ChainB).PauseMining(rctx)
		require.ErrorIs(t, err, harness.ErrCapabilityMissing,
			"harness must not advertise block control for a chain it did not launch")
	})

	// The subtest's harness env is now torn down. The out-of-band node must still answer: the harness
	// closed only its own client, it never signalled a node it did not launch. A fresh BlockNumber RPC on
	// the independently-owned client is a genuine liveness probe.
	afterHeight, err := oob.Height(ctx)
	require.NoError(t, err, "harness teardown must not have stopped the out-of-band external node")
	require.GreaterOrEqual(t, afterHeight, startHeight, "external node kept running across harness teardown")
}
