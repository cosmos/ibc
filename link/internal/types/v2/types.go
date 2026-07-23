// Package v2 contains shared IBC v2 domain types.
package v2

import (
	"time"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
)

// EventKind the kind of packet event.
type EventKind int

// Event kinds
const (
	KindUnknown EventKind = iota
	KindSendPacket
	KindWriteAck
)

// PacketEvent a packet event.
type PacketEvent struct {
	Height    uint64
	BlockTime time.Time
	Kind      EventKind
	Packet    channeltypesv2.Packet
	Acks      [][]byte
}
