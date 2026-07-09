// Package chains defines chain-agnostic clients for reading chain state.
package chains

import (
	"context"
	"time"
)

// Client provides chain-specific operations, implemented per chain type.
type Client interface {
	// TxPacketEvents parses packet events from the transaction's logs.
	TxPacketEvents(ctx context.Context, txHash []byte) ([]PacketEvent, error)
}

// EventKind the kind of packet event.
type EventKind int

// Event kinds
const (
	KindSendPacket EventKind = iota
	KindWriteAck
)

// Payload a single packet payload.
type Payload struct {
	SourcePort string
	DestPort   string
	Version    string
	Encoding   string
	Value      []byte
}

// Packet an IBC v2 packet.
type Packet struct {
	Sequence         uint64
	SourceClient     string
	DestClient       string
	TimeoutTimestamp uint64
	Payloads         []Payload
}

// PacketEvent a packet event observed on chain.
type PacketEvent struct {
	Height    uint64
	BlockTime time.Time
	Kind      EventKind
	Packet    Packet
	Acks      [][]byte
}
