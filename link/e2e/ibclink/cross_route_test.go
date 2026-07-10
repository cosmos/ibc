package ibclink_test

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

// crossRouteTopology shares destination chain-a; both routes send sequence 1 to probe bare-seq collision.
func crossRouteTopology() topology.Topology {
	spec := func(id string, chainID uint64) topology.ChainSpec {
		return topology.ChainSpec{
			Chain: wire.Chain{
				ID:           id,
				Type:         wire.ChainTypeEVM,
				Provider:     wire.ProviderAnvil,
				ChainID:      chainID,
				EVMSignerKey: testkeys.RelayerPrivateKeyHex,
			},
			Provision: topology.Provision{Mode: topology.ProvisionManaged, Launcher: topology.LauncherAnvil},
		}
	}
	route := func(id, src, dst string) wire.Route {
		return wire.Route{ID: id, Source: src, Destination: dst, Type: wire.RouteEVMToEVMAttested}
	}
	return topology.Topology{
		Name: "cross-route-anvil",
		Chains: []topology.ChainSpec{
			spec("chain-a", 31637),
			spec("chain-b", 31638),
			spec("chain-c", 31639),
		},
		Config: wire.ConfigYAML{Relayer: wire.Relayer{Routes: []wire.Route{
			route("b-to-a", "chain-b", "chain-a"),
			route("c-to-a", "chain-c", "chain-a"),
		}}},
	}
}

func TestCrossRoute_NoSequenceCollision(t *testing.T) {
	e2etest.RequireAnvilLane(t)
	run := e2etest.Start(t, crossRouteTopology())
	ctx := t.Context()

	bToA, err := run.IFT(ctx, harness.IFT{Route: "b-to-a", Amount: big.NewInt(333)})
	require.NoError(t, err)
	cToA, err := run.IFT(ctx, harness.IFT{Route: "c-to-a", Amount: big.NewInt(444)})
	require.NoError(t, err)

	require.NoError(t, bToA.VerifyComplete(ctx))
	require.NoError(t, cToA.VerifyComplete(ctx))
}
