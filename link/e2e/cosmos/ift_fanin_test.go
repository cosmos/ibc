package cosmos_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/e2e/e2etest"
	"github.com/cosmos/ibc/link/harness"
	"github.com/cosmos/ibc/link/harness/ibclink/wire"
	"github.com/cosmos/ibc/link/harness/testkeys"
	"github.com/cosmos/ibc/link/harness/topology"
)

func cosmosIFTSequenceCollisionTopology() topology.Topology {
	cosmosSpec := func(id string) topology.ChainSpec {
		return topology.ChainSpec{
			Chain: wire.Chain{
				ID:            id,
				Type:          wire.ChainTypeCosmos,
				Provider:      wire.ProviderSandbox,
				CosmosChainID: "sandbox-cosmos-1",
				SignerKey:     testkeys.CosmosSignerPrivateKeyHex,
				FaucetKey:     testkeys.CosmosFaucetPrivateKeyHex,
			},
			Provision: topology.Provision{Mode: topology.ProvisionManaged, Launcher: topology.LauncherSandbox},
		}
	}
	return topology.Topology{
		Name: "cosmos-ift-fan-in",
		Chains: []topology.ChainSpec{
			{
				Chain: wire.Chain{
					ID:           "chain-a",
					Type:         wire.ChainTypeEVM,
					Provider:     wire.ProviderAnvil,
					ChainID:      31647,
					EVMSignerKey: testkeys.RelayerPrivateKeyHex,
				},
				Provision: topology.Provision{Mode: topology.ProvisionManaged, Launcher: topology.LauncherAnvil},
			},
			cosmosSpec("chain-b"),
			cosmosSpec("chain-c"),
		},
		Config: wire.ConfigYAML{Relayer: wire.Relayer{Routes: []wire.Route{
			{ID: "b-to-a", Source: "chain-b", Destination: "chain-a", Type: wire.RouteCosmosToEVMAttested},
			{ID: "c-to-a", Source: "chain-c", Destination: "chain-a", Type: wire.RouteCosmosToEVMAttested},
		}}},
	}
}

// TestIFTTransfer_CosmosFanIn_NoSequenceCollision proves two native sources can both deliver their first
// packet to one EVM fixture without the destination replay guard cross-matching the raw sequence 1.
func TestIFTTransfer_CosmosFanIn_NoSequenceCollision(t *testing.T) {
	e2etest.RequireAnvilLane(t)
	e2etest.RequireSandboxd(t)

	run := e2etest.Start(t, cosmosIFTSequenceCollisionTopology())
	ctx := t.Context()

	bToA, err := run.IFT(ctx, harness.IFT{Route: "b-to-a", Amount: big.NewInt(333)})
	require.NoError(t, err)
	cToA, err := run.IFT(ctx, harness.IFT{Route: "c-to-a", Amount: big.NewInt(444)})
	require.NoError(t, err)

	require.NoError(t, bToA.VerifyComplete(ctx))
	require.NoError(t, cToA.VerifyComplete(ctx))
}
