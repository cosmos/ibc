package topology

import (
	"github.com/cosmos/ibc/link/harness/ibclink/wire"
)

const (
	// ChainA is the first logical chain id.
	ChainA = "chain-a"
	// ChainB is the second logical chain id.
	ChainB = "chain-b"
	// RouteAtoB is the directed route id from ChainA to ChainB.
	RouteAtoB = "route-a-to-b"
	// RouteBtoA is the directed route id from ChainB to ChainA.
	RouteBtoA = "route-b-to-a"
)

// Shape is the family-level arrangement of a two-chain topology: which chain family each of the
// canonical ChainA/ChainB slots holds, with both directed routes' types derived from the family
// pair. It is the only construction vocabulary tests touch — providers, launchers, timing, and
// chain ids come from the Lane that binds it. Fields are unexported so a usable Shape comes only
// from a constructor (binding the zero Shape panics): the vocabulary is deliberately closed at two
// chains, and an N-chain or otherwise
// novel arrangement starts as ad-hoc ChainSpec composition guarded by Topology.Validate, earning
// a constructor only once more than one call site needs it.
type Shape struct {
	name string
	// families is each slot's chain family, in ChainA, ChainB order.
	families [2]string
}

// TwoEVM is ChainA(EVM) <-> ChainB(EVM): two directed evmToEvmAttested routes.
func TwoEVM() Shape {
	return Shape{name: "two-evm", families: [2]string{wire.ChainTypeEVM, wire.ChainTypeEVM}}
}
