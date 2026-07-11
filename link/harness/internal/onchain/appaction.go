package onchain

import (
	"math/big"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"
)

type PacketAction struct {
	RouteID      string
	Source       string
	Destination  string
	SourceTxHash string
	Sequence     uint64
}

type IFTAction struct {
	PacketAction

	Receiver string
	Amount   *big.Int
}

func (a IFTAction) ID() string { return wire.PacketID(a.RouteID, wire.AppTypeIFT, a.Sequence) }

func (a *IFTAction) packet() *PacketAction { return &a.PacketAction }

type GMPAction struct {
	PacketAction

	Target string
}

func (a GMPAction) ID() string { return wire.PacketID(a.RouteID, wire.AppTypeGMP, a.Sequence) }

func (a *GMPAction) packet() *PacketAction { return &a.PacketAction }
