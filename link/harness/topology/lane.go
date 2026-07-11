package topology

import (
	"fmt"
	"time"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"
	"github.com/cosmos/ibc/link/harness/testkeys"
)

// Lane binds a Shape to concrete infrastructure: the per-slot provider/launcher assignment, timing
// profile, and chain ids. One exported Lane exists per E2E_LANE value — the runner picks the lane,
// the test picks the shape.
type Lane func(Shape) Topology

// Reserved chain-id ranges. Each lane spaces its EVM chain ids from its own base (+1 per additional
// EVM slot) so worlds from different lanes never share a chain id. An ad-hoc composed topology must pick
// its ids outside these ranges — Validate
// only checks uniqueness within one topology, not across coexisting worlds (cross_route_test.go uses
// 31637–31639; external_test.go pairs anvil's default 31337 with 31347).
const (
	// anvilChainIDBase is Anvil's default chain id, the instant-anvil lane's base.
	anvilChainIDBase = 31337
	// anvilIntervalChainIDBase spaces the interval-mining lane away from the instant one.
	anvilIntervalChainIDBase = 31437
	// besuChainIDBase is the Besu QBFT lane's base.
	besuChainIDBase = 32337
	// anvilIntervalBlockTime is the block cadence of the interval-mining anvil lane. It is the single
	// source of both the node's --block-time (via the resolved TimingProfile.BlockInterval) and every
	// wait budget the lane derives (via blockTiming), so the chain mines and the harness waits at one
	// rate. 2s is slow enough to genuinely exercise the non-instant path (a couple of blocks per packet
	// leg) while keeping the lane fast.
	anvilIntervalBlockTime = 2 * time.Second
)

// Anvil is the instant-mining lane: every EVM slot is a managed on-demand-mining Anvil — the only
// lane whose chains advertise block control and fault injection (pause/mine/advance-time,
// stop/restart).
func Anvil(s Shape) Topology {
	return bindEVMSlots(s, "anvil", func(id string, nth uint64) ChainSpec {
		return anvilSpec(id, anvilChainIDBase+nth)
	})
}

// AnvilInterval is the fixed-cadence lane: every EVM slot is a managed Anvil mining on a 2s block
// interval. It is the portable lane's proof that every packet-wait and reader budget derives from a
// per-chain timing profile rather than an instant-mining assumption. Block-control-dependent tests
// skip here — --block-time disables on-demand mining.
func AnvilInterval(s Shape) Topology {
	return bindEVMSlots(s, "anvil-interval", func(id string, nth uint64) ChainSpec {
		spec := anvilSpec(id, anvilIntervalChainIDBase+nth)
		spec.Timing = blockTiming(anvilIntervalBlockTime)
		return spec
	})
}

// Besu is the dockerized Besu QBFT lane: every EVM slot is a managed Besu node (timing resolves to
// the QBFT block-period profile via DefaultTiming).
func Besu(s Shape) Topology {
	return bindEVMSlots(s, "besu", func(id string, nth uint64) ChainSpec {
		return besuSpec(id, besuChainIDBase+nth)
	})
}

// bindEVMSlots binds every slot of the shape through evmSpec; nth spaces the lane's chain ids.
func bindEVMSlots(s Shape, lane string, evmSpec func(id string, nth uint64) ChainSpec) Topology {
	ids := [2]string{ChainA, ChainB}
	specs := make([]ChainSpec, 0, len(s.families))
	for i := range s.families {
		specs = append(specs, evmSpec(ids[i], uint64(i)))
	}
	return bind(s, lane, specs)
}

// bind assembles a lane's bound Topology: one ChainSpec per shape slot plus the shape's two directed
// routes with types derived from the slot families, named <shape>-<lane> (e.g. two-evm-anvil) so
// artifacts and logs stay greppable per cell; derivations append their own suffixes (+manual).
func bind(s Shape, lane string, specs []ChainSpec) Topology {
	if s.name == "" {
		panic("topology: zero Shape — construct one with TwoEVM")
	}
	return Topology{
		Name:   s.name + "-" + lane,
		Chains: specs,
		Config: wire.ConfigYAML{Relayer: wire.Relayer{Routes: []wire.Route{
			{ID: RouteAtoB, Source: ChainA, Destination: ChainB, Type: mustRouteType(s.families[0], s.families[1])},
			{ID: RouteBtoA, Source: ChainB, Destination: ChainA, Type: mustRouteType(s.families[1], s.families[0])},
		}}},
	}
}

// mustRouteType is wire.RouteTypeFor for shape binding, where every family pair relays by
// construction — a miss means a new Shape constructor paired families no route type covers.
func mustRouteType(srcFamily, dstFamily string) string {
	rt, ok := wire.RouteTypeFor(srcFamily, dstFamily)
	if !ok {
		panic(fmt.Sprintf("topology: no route type relays %s -> %s", srcFamily, dstFamily))
	}
	return rt
}

// anvilSpec is one managed instant-Anvil EVM slot. Timing stays zero (instant profile) unless the
// lane sets it.
func anvilSpec(id string, chainID uint64) ChainSpec {
	return ChainSpec{
		Chain: wire.Chain{
			ID:           id,
			Type:         wire.ChainTypeEVM,
			Provider:     wire.ProviderAnvil,
			ChainID:      chainID,
			EVMSignerKey: testkeys.RelayerPrivateKeyHex,
		},
		Provision: Provision{Mode: ProvisionManaged, Launcher: LauncherAnvil},
	}
}

// besuSpec is one managed dockerized Besu QBFT EVM slot.
func besuSpec(id string, chainID uint64) ChainSpec {
	return ChainSpec{
		Chain: wire.Chain{
			ID:           id,
			Type:         wire.ChainTypeEVM,
			Provider:     wire.ProviderBesu,
			ChainID:      chainID,
			EVMSignerKey: testkeys.RelayerPrivateKeyHex,
		},
		Provision: Provision{Mode: ProvisionManaged, Launcher: LauncherBesu},
	}
}
