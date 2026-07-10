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

// crossRouteTopology is three managed Anvil chains with two directed evmToEvmAttested routes that share the
// destination chain-a: b->a and c->a. Each source's own MockIFT fixture assigns sequences starting at 1, so
// the first transfer over each route both carry sequence 1 onto chain-a's single MockIFT — colliding
// anywhere packet identity drops the route (destination idempotency, the reader's event match).
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

// TestCrossRoute_NoSequenceCollision locks the invariant that packet identity carries a route
// discriminator (fixturekeys.RouteScopedSeq): sequences are only unique per source fixture, so identity keyed
// on a bare sequence cross-matches routes that share a destination. It sends one IFT over each of two
// routes sharing destination chain-a (b->a and c->a) with distinct receivers and amounts; both sources
// assign sequence 1. If any layer keys on the bare sequence, the second delivery cross-matches the
// first on chain-a's fixture — marked complete with the first's tx, its own mint never landing.
// VerifyComplete asserts each mint credited the action's own receiver its own amount (a cross-matched or
// suppressed delivery fails here), so both passing proves neither route cross-contaminated the other.
// Anvil-lane only (it pins a three-chain Anvil topology regardless of the selected lane).
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
