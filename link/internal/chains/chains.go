// Package chains defines chain-agnostic packet event parsing.
package chains

import (
	"context"
	"time"
)

// Client parses packet details out of transaction event logs.
// There is an implementation per supported chain type.
type Client interface {
	TxPacketEvents(ctx context.Context, txHashes [][]byte) ([]PacketEvent, error)
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

// Registry resolves chain clients by chain id.
type Registry struct {
	clients map[string]Client
}

// NewRegistry Registry constructor.
func NewRegistry() *Registry {
	return &Registry{clients: make(map[string]Client)}
}

// Add registers a client for a chain id.
func (r *Registry) Add(chainID string, client Client) {
	r.clients[chainID] = client
}

// Client returns the client for a chain id.
func (r *Registry) Client(chainID string) (Client, bool) {
	client, ok := r.clients[chainID]
	return client, ok
}

type closer interface {
	Close()
}

// Close closes all clients that support closing.
func (r *Registry) Close() error {
	for _, client := range r.clients {
		if c, ok := client.(closer); ok {
			c.Close()
		}
	}

	return nil
}
