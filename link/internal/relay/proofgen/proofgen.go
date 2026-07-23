// Package proofgen generates packet membership/non-membership proofs and
// light-client state proofs. There is one implementation per light-client
// type.
package proofgen

import (
	"context"
	"time"

	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

// ProofKind the kind of packet claim a proof attests to.
type ProofKind int

// Proof kinds
const (
	KindUnknown ProofKind = iota
	KindPacketCommitment
	KindAcknowledgement
	KindReceiptAbsence
)

// ProofGenerator generates packet membership/non-membership proofs and state
// proofs for one configured light client.
type ProofGenerator interface {
	// LatestProvableHeight resolves the highest height a subsequent StateProof
	// and PacketProofs call sharing that height can currently succeed at,
	// along with that height's counterparty-chain timestamp: the two proofs
	// must agree on height for on-chain verification to succeed, so callers
	// resolve it once and pass the same value to both. Callers filter their
	// own batch down to what's provable at the returned height/timestamp
	// (e.g. packets observed at or before this height, or timeouts at or
	// before this timestamp) rather than passing in a requirement, since
	// demanding a specific height/timestamp up front would fail the whole
	// batch if even one item in it isn't provable yet.
	LatestProvableHeight(ctx context.Context) (uint64, time.Time, error)

	// StateProof proves the light client's counterparty state at height.
	StateProof(ctx context.Context, height uint64) ([]byte, error)

	// PacketProofs proves each packet's membership or non-membership at
	// height, one proof per packet with indices aligned to packets. Returns
	// an error if a proof cannot be generated for any packet, rather than
	// silently omitting it from the result.
	PacketProofs(ctx context.Context, height uint64, kind ProofKind, packets []v2.Packet) ([][]byte, error)
}

// Key identifies one configured light client by the chain it lives on and
// its client id, the composite key ProofGenerator instances are scoped by.
func Key(chainID, clientID string) string {
	return chainID + "/" + clientID
}

// Set resolves a ProofGenerator by (chainID, clientID).
type Set struct {
	generators map[string]ProofGenerator
}

func NewSet(generators map[string]ProofGenerator) *Set {
	if generators == nil {
		generators = make(map[string]ProofGenerator)
	}

	return &Set{generators: generators}
}

func (s *Set) Get(chainID, clientID string) (ProofGenerator, bool) {
	generator, ok := s.generators[Key(chainID, clientID)]
	return generator, ok
}
