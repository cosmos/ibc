package topology

import (
	"fmt"
	"time"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"
	"github.com/cosmos/ibc/link/harness/testkeys"
)

type Lane func(Shape) Topology

// Lane chain ID ranges must not overlap; ad-hoc topologies must use IDs outside them.
const (
	anvilChainIDBase         = 31337
	anvilIntervalChainIDBase = 31437
	besuChainIDBase          = 32337
	anvilIntervalBlockTime   = 2 * time.Second
)

func Anvil(s Shape) Topology {
	return bindEVMSlots(s, "anvil", func(id string, nth uint64) ChainSpec {
		return anvilSpec(id, anvilChainIDBase+nth)
	})
}

func AnvilInterval(s Shape) Topology {
	return bindEVMSlots(s, "anvil-interval", func(id string, nth uint64) ChainSpec {
		spec := anvilSpec(id, anvilIntervalChainIDBase+nth)
		spec.Timing = blockTiming(anvilIntervalBlockTime)
		return spec
	})
}

func Besu(s Shape) Topology {
	return bindEVMSlots(s, "besu", func(id string, nth uint64) ChainSpec {
		return besuSpec(id, besuChainIDBase+nth)
	})
}

func bindEVMSlots(s Shape, lane string, evmSpec func(id string, nth uint64) ChainSpec) Topology {
	ids := [2]string{ChainA, ChainB}
	specs := make([]ChainSpec, 0, len(s.families))
	for i := range s.families {
		specs = append(specs, evmSpec(ids[i], uint64(i)))
	}
	return bind(s, lane, specs)
}

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

func mustRouteType(srcFamily, dstFamily string) string {
	rt, ok := wire.RouteTypeFor(srcFamily, dstFamily)
	if !ok {
		panic(fmt.Sprintf("topology: no route type relays %s -> %s", srcFamily, dstFamily))
	}
	return rt
}

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
