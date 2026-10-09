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

// OperationType identifies the action performed on a packet.
type OperationType int

// Operation types
const (
	OperationTypeUnknown OperationType = iota
	OperationTypeRecv
	OperationTypeAck
	OperationTypeTimeout
)

func (t OperationType) String() string {
	switch t {
	case OperationTypeRecv:
		return "recv"
	case OperationTypeAck:
		return "ack"
	case OperationTypeTimeout:
		return "timeout"
	default:
		return "unknown"
	}
}

// PacketRelayItem one packet operation to include in a relay tx, along with
// the membership/non-membership proof authorizing it.
type PacketRelayItem struct {
	Type        OperationType
	Packet      channeltypesv2.Packet
	Acks        [][]byte // one per payload, order preserved; only set for OperationTypeAck
	Proof       []byte
	ProofHeight uint64
}

// ClientUpdate the payloads to update a destination client with, in order,
// before any packet operations in the same tx are processed.
type ClientUpdate struct {
	ClientID string
	Payloads [][]byte // Empty when no update is needed.
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

// TxIntent describes a transaction and optional operation metadata.
type TxIntent struct {
	Operations []OperationMetadata
	To         string
	Data       []byte
}

// OperationMetadata identifies a packet and the relay action to perform on it.
type OperationMetadata struct {
	SourceChainID  string
	SourceClientID string
	Sequence       uint64
	Type           OperationType
}
