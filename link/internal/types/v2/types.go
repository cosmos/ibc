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
	Packet    channeltypesv2.Packet
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
