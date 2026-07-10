// Package external_test covers harness connectivity to an out-of-band chain.
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

// externalChainID differs from managedChainID so live validate exercises chain-id on a second node.
const (
	managedChainID  = 31337
	externalChainID = 31347
)

func TestExternalChain_HarnessConnectsButDoesNotOwn(t *testing.T) {
	e2etest.RequireAnvilLane(t)
	ctx := t.Context()

	oob, err := anvil.Start(ctx, anvil.Spec{
		ID:      "chain-b-external",
		ChainID: externalChainID,
		LogPath: filepath.Join(t.TempDir(), "external-anvil.log"),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = oob.Stop() })

	startHeight, err := oob.Height(ctx)
	require.NoError(t, err)

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

	// Subtest teardown must finish before the out-of-band liveness probe below, or the check is vacuous.
	t.Run("harness", func(t *testing.T) {
		run := e2etest.Start(t, topo)
		rctx := t.Context()

		out, err := run.IFT(rctx, harness.IFT{Route: topology.RouteAtoB, Amount: big.NewInt(1_500_000)})
		require.NoError(t, err)
		require.NoError(t, out.VerifyComplete(rctx))

		err = run.Chain(topology.ChainB).PauseMining(rctx)
		require.ErrorIs(t, err, harness.ErrCapabilityMissing,
			"harness must not advertise block control for a chain it did not launch")
	})

	afterHeight, err := oob.Height(ctx)
	require.NoError(t, err, "harness teardown must not have stopped the out-of-band external node")
	require.GreaterOrEqual(t, afterHeight, startHeight, "external node kept running across harness teardown")
}
