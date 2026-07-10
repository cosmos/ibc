package topology

import (
	"fmt"
	"time"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"
	"github.com/cosmos/ibc/link/harness/testkeys"
)

// Lane binds a Shape to concrete infrastructure: the per-slot provider/launcher assignment, timing
// profile, and chain ids. One exported Lane exists per E2E_LANE value — the runner picks the lane,
// the test picks the shape. A lane is not a family→provider map: the sandbox lane is deliberately
// heterogeneous (anvil on one slot, sandboxd on the other), so each lane assigns providers per slot.
type Lane func(Shape) Topology

// Reserved chain-id ranges. Each lane spaces its EVM chain ids from its own base (+1 per additional
// EVM slot) so worlds from different lanes never share a chain id; the sandbox chain's id is fixed
// by the node itself. An ad-hoc composed topology must pick its ids outside these ranges — Validate
// only checks uniqueness within one topology, not across coexisting worlds (cross_route_test.go uses
// 31637–31639; external_test.go pairs anvil's default 31337 with 31347).
const (
	// anvilChainIDBase is Anvil's default chain id, the instant-anvil lane's base.
	anvilChainIDBase = 31337
	// anvilIntervalChainIDBase spaces the interval-mining lane away from the instant one.
	anvilIntervalChainIDBase = 31437
	// besuChainIDBase is the Besu QBFT lane's base.
	besuChainIDBase = 32337
	// sandboxEVMChainID is the managed sandboxd chain's numeric EVM id (the reference localnet's
	// default). It is deliberately far from the Anvil ids so the sandbox lane's two chains never
	// collide and the config's per-chain chain-id check is exercised against a genuinely different node.
	sandboxEVMChainID = 19460

	// sandboxCosmosChainID is the CometBFT chain-id string of a managed cosmos sandbox chain. It is what
	// the node is `init`ed with AND what the stub signs its SignDoc with, so it is the single source both
	// derive from (set on the wire.Chain, projected into the ibc link config by Compile, and passed to
	// the node launcher by provisioning).
	sandboxCosmosChainID = "sandbox-cosmos-1"

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

// Sandbox is the real-node lane: ChainA stays an instant Anvil and ChainB is a managed sandboxd
// node (a real Cosmos SDK + cosmos/evm chain), presented as whatever family the shape gives the
// slot — eth JSON-RPC for an EVM slot, CometBFT RPC + bank + gRPC for a cosmos one. It proves the
// harness relays across a genuinely different node (real consensus) while the ibc link config still
// describes ordinary chains; sandboxd is neither a BlockController nor a FaultInjector, so
// block-control tests skip here.
func Sandbox(s Shape) Topology {
	last := sandboxCosmosSpec(ChainB)
	if s.families[1] == wire.ChainTypeEVM {
		last = sandboxEVMSpec(ChainB)
	}
	return bind(s, "sandbox", []ChainSpec{anvilSpec(ChainA, anvilChainIDBase), last})
}

// bindEVMSlots binds every slot of the shape for one lane: the nth EVM slot (slot order) through
// evmSpec — nth spaces the lane's chain ids — and every cosmos slot to managed sandboxd, the only
// cosmos provider. A cosmos slot's Timing stays zero so it resolves to DefaultTiming(LauncherSandbox).
func bindEVMSlots(s Shape, lane string, evmSpec func(id string, nth uint64) ChainSpec) Topology {
	ids := [2]string{ChainA, ChainB}
	specs := make([]ChainSpec, 0, len(s.families))
	nth := uint64(0)
	for i, family := range s.families {
		if family == wire.ChainTypeCosmos {
			specs = append(specs, sandboxCosmosSpec(ids[i]))
			continue
		}
		specs = append(specs, evmSpec(ids[i], nth))
		nth++
	}
	return bind(s, lane, specs)
}

// bind assembles a lane's bound Topology: one ChainSpec per shape slot plus the shape's two directed
// routes with types derived from the slot families, named <shape>-<lane> (e.g. two-evm-anvil) so
// artifacts and logs stay greppable per cell; derivations append their own suffixes (+manual).
func bind(s Shape, lane string, specs []ChainSpec) Topology {
	if s.name == "" {
		panic("topology: zero Shape — construct one with TwoEVM or EVMCosmos")
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

// sandboxEVMSpec is one managed sandboxd slot presented as EVM (eth JSON-RPC): the ibc link config
// sees an ordinary EVM chain; only Provision and the node behind it differ.
func sandboxEVMSpec(id string) ChainSpec {
	return ChainSpec{
		Chain: wire.Chain{
			ID:           id,
			Type:         wire.ChainTypeEVM,
			Provider:     wire.ProviderSandbox,
			ChainID:      sandboxEVMChainID,
			EVMSignerKey: testkeys.RelayerPrivateKeyHex,
		},
		Provision: Provision{Mode: ProvisionManaged, Launcher: LauncherSandbox},
	}
}

// sandboxCosmosSpec is one managed sandboxd slot presented as a cosmos chain (CometBFT RPC + bank +
// bech32 accounts). A cosmos chain carries no numeric EVM ChainID — its identity is the cosmos
// chain-id string. SignerKey is the relayer/admin signing credential; FaucetKey is the user/faucet
// account a Cosmos-source transfer burns from; the gRPC URL is left blank and
// bound at compile from the runtime bindings.
func sandboxCosmosSpec(id string) ChainSpec {
	return ChainSpec{
		Chain: wire.Chain{
			ID:            id,
			Type:          wire.ChainTypeCosmos,
			Provider:      wire.ProviderSandbox,
			CosmosChainID: sandboxCosmosChainID,
			SignerKey:     testkeys.CosmosSignerPrivateKeyHex,
			FaucetKey:     testkeys.CosmosFaucetPrivateKeyHex,
		},
		Provision: Provision{Mode: ProvisionManaged, Launcher: LauncherSandbox},
	}
}
