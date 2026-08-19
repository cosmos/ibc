// SPDX-License-Identifier: Apache-2.0

// Package lightclient defines custom light-client proof generation.
package lightclient

import (
	"context"
	"time"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
)

// Prover generates proofs for one light client.
type Prover interface {
	// LatestProvableHeight returns the latest provable height and timestamp.
	LatestProvableHeight(ctx context.Context) (uint64, time.Time, error)

	// StateProof proves counterparty state at height.
	StateProof(ctx context.Context, height uint64) ([]byte, error)

	// PacketProofs returns one proof per packet, in packet order.
	PacketProofs(
		ctx context.Context,
		height uint64,
		kind ProofKind,
		packets []channeltypesv2.Packet,
	) ([][]byte, error)
}

// ProofKind identifies the packet claim being proved.
type ProofKind int

const (
	ProofKindUnknown ProofKind = iota
	ProofKindPacketCommitment
	ProofKindAcknowledgement
	ProofKindReceiptAbsence
)
