// SPDX-License-Identifier: Apache-2.0

// Package v2 contains shared IBC v2 domain types.
package v2

import (
	"time"

	"github.com/pkg/errors"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	hostv2 "github.com/cosmos/ibc-go/v11/modules/core/24-host/v2"
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

// ProofKind the kind of packet claim a proof attests to.
type ProofKind int

// Proof kinds
const (
	ProofKindUnknown ProofKind = iota
	ProofKindPacketCommitment
	ProofKindAcknowledgement
	ProofKindReceiptAbsence
)

// CommitmentPath is the raw commitment path a proof of kind covers, keyed by
// the client the proven chain stores it under.
func (k ProofKind) CommitmentPath(packet channeltypesv2.Packet) ([]byte, error) {
	switch k {
	case ProofKindPacketCommitment:
		return hostv2.PacketCommitmentKey(packet.SourceClient, packet.Sequence), nil
	case ProofKindAcknowledgement:
		return hostv2.PacketAcknowledgementKey(packet.DestinationClient, packet.Sequence), nil
	case ProofKindReceiptAbsence:
		return hostv2.PacketReceiptKey(packet.DestinationClient, packet.Sequence), nil
	default:
		return nil, errors.Errorf("unsupported proof kind %d", k)
	}
}

// RelayKind the packet operation one PacketRelayItem asks to perform.
type RelayKind int

// Relay kinds
const (
	RelayKindUnknown RelayKind = iota
	RelayKindRecv
	RelayKindAck
	RelayKindTimeout
)

func (k RelayKind) String() string {
	switch k {
	case RelayKindRecv:
		return "recv"
	case RelayKindAck:
		return "ack"
	case RelayKindTimeout:
		return "timeout"
	default:
		return "unknown"
	}
}

// PacketRelayItem one packet operation to include in a relay tx, along with
// the membership/non-membership proof authorizing it.
type PacketRelayItem struct {
	Kind        RelayKind
	Packet      channeltypesv2.Packet
	Acks        [][]byte // one per payload, order preserved; only set for RelayKindAck
	Proof       []byte
	ProofHeight uint64
}

// ClientUpdate the payload to update a destination client with before
// any packet operations in the same tx are processed.
type ClientUpdate struct {
	ClientID string
	Payload  []byte // Empty when no update is needed.
}

// RelayTx one transaction ready to submit, targeting To with calldata Data.
type RelayTx struct {
	To   []byte
	Data []byte
}

// PacketEvent a packet event.
type PacketEvent struct {
	Height    uint64
	BlockTime time.Time
	Kind      EventKind
	Packet    channeltypesv2.Packet
	Acks      [][]byte
	TxHash    string
	// Removed the event's block was reorged out. Always false on an
	// instant-finality chain.
	Removed bool
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
