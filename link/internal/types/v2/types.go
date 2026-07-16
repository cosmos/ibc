// Package v2 contains shared IBC v2 domain types.
package v2

import "time"

// EventKind the kind of packet event.
type EventKind int

// Event kinds
const (
	KindUnknown EventKind = iota
	KindSendPacket
	KindWriteAck
)

// Payload a packet payload.
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

// WriteAckStatus the result of a packet's write acknowledgement.
type WriteAckStatus int

// Write ack statuses
const (
	WriteAckStatusUnknown WriteAckStatus = iota
	WriteAckStatusSuccess
	WriteAckStatusError
)

// PacketEvent a packet event.
type PacketEvent struct {
	Height    uint64
	BlockTime time.Time
	Kind      EventKind
	Packet    Packet
	Acks      [][]byte
}

// Tx a transaction observed on a chain.
type Tx struct {
	Hash           string
	Timestamp      time.Time
	RelayerAddress string
}

// TxIntent a transaction for the relayer to submit.
type TxIntent struct {
	To   string
	Data []byte
}

// Submission a transaction broadcast by the relayer.
type Submission struct {
	TxHash         string
	SubmittedAt    time.Time
	RelayerAddress string
}
