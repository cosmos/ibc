// SPDX-License-Identifier: Apache-2.0

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

// ProofKind the kind of packet claim a proof attests to.
type ProofKind int

// Proof kinds
const (
	ProofKindUnknown ProofKind = iota
	ProofKindPacketCommitment
	ProofKindAcknowledgement
	ProofKindReceiptAbsence
)

// RelayKind the packet operation one PacketRelayItem asks to perform.
type RelayKind int

// Relay kinds
const (
	RelayKindUnknown RelayKind = iota
	RelayKindRecv
	RelayKindAck
	RelayKindTimeout
)

// PacketRelayItem one packet operation to include in a relay tx, along with
// the membership/non-membership proof authorizing it.
type PacketRelayItem struct {
	Kind        RelayKind
	Packet      channeltypesv2.Packet
	Acks        [][]byte // one per payload, order preserved; only set for RelayKindAck
	Proof       []byte
	ProofHeight uint64
}

// ClientUpdate the state proof to update a destination client with before
// any packet operations in the same tx are processed.
type ClientUpdate struct {
	ClientID   string
	StateProof []byte
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
