package onchain

import (
	"math/big"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"
)

// PacketAction is the common correlation data produced by an app submitter.
type PacketAction struct {
	RouteID      string
	Source       string
	Destination  string
	SourceTxHash string
	Sequence     uint64
}

// IFTAction is a submitted IFT transfer. Amount is owned by the action.
type IFTAction struct {
	PacketAction

	Receiver string
	Amount   *big.Int
}

// ID derives the IFT packet ID from the action's coordinates.
func (a IFTAction) ID() string { return wire.PacketID(a.RouteID, wire.AppTypeIFT, a.Sequence) }

func (a *IFTAction) packet() *PacketAction { return &a.PacketAction }

// GMPAction is a submitted GMP call: the shared correlation data plus the target the harness addressed.
type GMPAction struct {
	PacketAction

	Target string
}

// ID derives the GMP packet ID from the action's coordinates.
func (a GMPAction) ID() string { return wire.PacketID(a.RouteID, wire.AppTypeGMP, a.Sequence) }

func (a *GMPAction) packet() *PacketAction { return &a.PacketAction }
