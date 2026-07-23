// Package txbuilder assembles transactions from packet relay and client
// update details. There is one implementation per supported chain type.
package txbuilder

import channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"

// RelayKind the packet operation one PacketRelayItem asks to perform.
type RelayKind int

// Relay kinds
const (
	KindUnknown RelayKind = iota
	KindRecv
	KindAck
	KindTimeout
)

// PacketRelayItem one packet operation to include in a relay tx, along with
// the membership/non-membership proof authorizing it.
type PacketRelayItem struct {
	Kind        RelayKind
	Packet      channeltypesv2.Packet
	Acks        [][]byte // one per payload, order preserved; only set for KindAck
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

// TxBuilder builds transactions from packet relay and client update details
// for one chain. It returns a list of transactions rather than one, since
// some chains (e.g. Solana, due to tx size limits) must split a batch of
// packet relays across multiple transactions.
type TxBuilder interface {
	BuildRelayTxs(clientUpdate ClientUpdate, packetRelayItems []PacketRelayItem) ([]RelayTx, error)
}

// Set resolves a TxBuilder by chain id.
type Set struct {
	builders map[string]TxBuilder
}

func NewSet(builders map[string]TxBuilder) *Set {
	if builders == nil {
		builders = make(map[string]TxBuilder)
	}

	return &Set{builders: builders}
}

func (s *Set) Get(chainID string) (TxBuilder, bool) {
	builder, ok := s.builders[chainID]
	return builder, ok
}
