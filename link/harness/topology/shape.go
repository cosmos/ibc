package topology

import (
	"github.com/cosmos/ibc/link/harness/ibclink/wire"
)

const (
	ChainA    = "chain-a"
	ChainB    = "chain-b"
	RouteAtoB = "route-a-to-b"
	RouteBtoA = "route-b-to-a"
)

type Shape struct {
	name     string
	families [2]string
}

func TwoEVM() Shape {
	return Shape{name: "two-evm", families: [2]string{wire.ChainTypeEVM, wire.ChainTypeEVM}}
}
