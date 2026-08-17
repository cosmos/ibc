// SPDX-License-Identifier: Apache-2.0

// Package proofgen is the public extension point for light client proof
// generation. Implement ProofGenerator and Factory here, register the factory
// under a client type name, and start a relayer with it via link/app.
//
// This package must not import anything under link/internal. Everything it
// exposes is deliberately narrow: proofs are opaque bytes, and the relayer
// never interprets them.
package proofgen

import (
	"context"
	"time"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
)

// ProofGenerator generates packet membership/non-membership proofs and state
// proofs for one configured light client.
type ProofGenerator interface {
	// LatestProvableHeight resolves the highest height a subsequent StateProof
	// and PacketProofs call sharing that height can currently succeed at,
	// along with that height's counterparty-chain timestamp.
	LatestProvableHeight(ctx context.Context) (uint64, time.Time, error)

	// StateProof proves the light client's counterparty state at height. The
	// returned bytes are passed to the light client's updateClient entrypoint
	// unmodified.
	StateProof(ctx context.Context, height uint64) ([]byte, error)

	// PacketProofs proves each packet's membership or non-membership at
	// height, one proof per packet with indices aligned to packets. Returns
	// an error if a proof cannot be generated for any packet.
	PacketProofs(
		ctx context.Context,
		height uint64,
		kind ProofKind,
		packets []channeltypesv2.Packet,
	) ([][]byte, error)
}

// ProofKind the kind of packet claim a proof attests to.
type ProofKind int

// Proof kinds.
const (
	ProofKindUnknown ProofKind = iota
	ProofKindPacketCommitment
	ProofKindAcknowledgement
	ProofKindReceiptAbsence
)

// BlockHeader is the minimal view of a counterparty block a proof generator
// needs to reason about heights and timestamps.
type BlockHeader struct {
	Height    uint64
	Timestamp time.Time
}

// CounterpartyChain reads state from the chain a light client tracks. It is a
// deliberately narrow view of the relayer's full chain client; it may gain
// methods over time, which is safe because implementations live in the relayer
// and generators only consume it.
type CounterpartyChain interface {
	ChainID() string

	// GetBlockHeader returns the header at height. The special height markers
	// used internally by the relayer are not part of this contract; pass a
	// concrete height.
	GetBlockHeader(ctx context.Context, height uint64) (BlockHeader, error)
}

// Deps are the relayer-provided dependencies handed to a Factory.
type Deps struct {
	// Counterparty reads the chain this client tracks.
	Counterparty CounterpartyChain
}

// ClientEnd is one side of a configured connection: a light client on ChainID,
// tracking the connection's other end as its counterparty.
type ClientEnd struct {
	ChainID  string
	ClientID string
	Type     string
	Params   *RawParams
}
