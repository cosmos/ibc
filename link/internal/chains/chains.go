// Package chains defines chain-agnostic packet event parsing.
package chains

import (
	"context"
	"sync"
	"time"

	"github.com/pkg/errors"
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

// ClientManager holds the chain clients for all configured chains.
// Safe for concurrent use.
type ClientManager struct {
	mu      sync.RWMutex
	clients map[string]Client
}

// NewClientManager ClientManager constructor.
func NewClientManager(clients map[string]Client) *ClientManager {
	if clients == nil {
		clients = make(map[string]Client)
	}

	return &ClientManager{clients: clients}
}

// GetClient returns the chain client for a chain id.
func (m *ClientManager) GetClient(_ context.Context, chainID string) (Client, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	client, ok := m.clients[chainID]
	if !ok {
		return nil, errors.Errorf("no configured chain client for chain ID %s", chainID)
	}

	return client, nil
}
