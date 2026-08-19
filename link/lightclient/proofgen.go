// SPDX-License-Identifier: Apache-2.0

// Package lightclient is the public extension point for light client proof
// generation. Implement ProofGenerator and ProverFactory here, register the factory
// under a client type name, and supply it through link/cli.
//
// This package must not import anything under link/internal. Everything it
// exposes is deliberately narrow: proofs are opaque bytes, and the relayer
// never interprets them.
package lightclient

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

// ClientInfo describes one configured light-client instance from the
// perspective of the ProverFactory responsible for its type.
type ClientInfo struct {
	ChainID             string
	CounterpartyChainID string
	ClientID            string
	Type                string
	ClientParams        *RawParams
}

// ChainInfo is the chain configuration relevant to proof generation. It omits
// operational settings, such as the deployer, that custom provers do not need.
type ChainInfo struct {
	ChainID string
	EVM     *EVMChainInfo
}

// EVMChainInfo contains the EVM connection details available to a prover.
type EVMChainInfo struct {
	RPC         string
	ICS26Router string
}

// FactoryOptions describes the configured light-client instance a ProverFactory
// constructs. Additional shared construction dependencies may be added here
// as the extension API evolves.
type FactoryOptions struct {
	Client            ClientInfo
	HostChain         ChainInfo
	CounterpartyChain ChainInfo
}
