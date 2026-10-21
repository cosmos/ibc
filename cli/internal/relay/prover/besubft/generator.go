// SPDX-License-Identifier: Apache-2.0

// Package besubft implements prover.Prover for a Besu light client, QBFT or
// IBFT 2.0: it reads the client's state from the host chain, fetches sealed
// headers and eth_getProof results from the Besu chain it tracks and encodes
// update and membership payloads. The contract verifies everything they
// carry, checked by simulation at submission.
package besubft

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/cosmos/ibc-contracts/packages/go-abigen/besumsgs"
	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	"github.com/cosmos/ibc/cli/besu"
	"github.com/cosmos/ibc/cli/internal/chains/evm"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

// Host is the chain the light client lives on. It needs no Besu consensus:
// any EVM chain hosting the contract qualifies.
type Host interface {
	BesuClientState(ctx context.Context, clientID string) (besumsgs.IBesuLightClientMsgsClientState, error)
	// BesuConsensusStateHash fails when the client stores no consensus state
	// at height.
	BesuConsensusStateHash(ctx context.Context, clientID string, height uint64) (common.Hash, error)
}

// Counterparty is the Besu chain the light client tracks.
type Counterparty interface {
	ChainID() string
	RouterAddress() common.Address
	// SealedHeader returns the parsed header at height, or the head at
	// v2.LatestBlock.
	SealedHeader(ctx context.Context, height uint64) (*besu.ParsedHeader, error)
	GetRouterProof(ctx context.Context, height uint64, slots []common.Hash) (evm.RouterProof, error)
}

// Generator implements prover.Prover for one Besu light client.
type Generator struct {
	host         Host
	counterparty Counterparty
	clientID     string
	logger       *slog.Logger

	// trusted is the client's latest trusted header last read. A header at a
	// height never changes.
	trusted atomic.Pointer[besu.ParsedHeader]
	// reported is the height last logged as the furthest the client reaches.
	reported atomic.Uint64
}

// NewGenerator builds the prover for clientID on host, tracking counterparty,
// and fails fast unless the client proves counterparty's configured router.
func NewGenerator(
	ctx context.Context,
	clientID string,
	host Host,
	counterparty Counterparty,
	logger *slog.Logger,
) (*Generator, error) {
	state, err := host.BesuClientState(ctx, clientID)
	if err != nil {
		return nil, err
	}

	if state.IbcRouter != counterparty.RouterAddress() {
		return nil, fmt.Errorf(
			"client %q proves router %s but chain %s is configured with router %s",
			clientID, state.IbcRouter, counterparty.ChainID(), counterparty.RouterAddress(),
		)
	}

	return &Generator{host: host, counterparty: counterparty, clientID: clientID, logger: logger}, nil
}

// LatestProvableHeight returns the counterparty head, or the furthest header
// the prover finds the client can reach when a validator set change it cannot
// follow comes first. The light client rejects an update it cannot verify when
// the relay is simulated.
func (g *Generator) LatestProvableHeight(ctx context.Context) (uint64, time.Time, error) {
	head, err := g.counterparty.SealedHeader(ctx, v2.LatestBlock)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("reading counterparty chain head: %w", err)
	}

	state, err := g.host.BesuClientState(ctx, g.clientID)
	if err != nil {
		return 0, time.Time{}, err
	}
	if head.Height <= state.LatestHeight.RevisionHeight {
		return head.Height, blockTime(head), nil
	}

	trusted, err := g.trustedHeader(ctx, state.LatestHeight.RevisionHeight)
	if err != nil {
		return 0, time.Time{}, err
	}
	// Which headers the search finds depends on its target, so when it stops
	// short, a height is reported only once the search ClientUpdatePayloads
	// repeats towards it succeeds. Each retry stops lower, ending at the
	// trusted height at worst.
	reached := head
	var first *stuckError
	for reached.Height > trusted.Height {
		_, stuck, err := g.path(ctx, trusted, reached, state.TrustLevel)
		if err != nil {
			return 0, time.Time{}, err
		}
		if stuck == nil {
			break
		}
		if first == nil {
			first = stuck
		}
		reached = stuck.reached
	}

	if first != nil && g.reported.Swap(first.reached.Height) != first.reached.Height {
		g.logger.Error(
			"Client cannot follow a validator set change; relaying only up to the furthest height it reaches",
			"reachedHeight", reached.Height,
			"headHeight", head.Height,
			"err", first,
		)
	}
	return reached.Height, blockTime(reached), nil
}

func blockTime(header *besu.ParsedHeader) time.Time {
	return time.Unix(int64(header.Timestamp), 0).UTC() //nolint:gosec // block times fit in int64
}

// ClientUpdatePayloads returns the updateMsgs from the client's latest
// consensus state to target, each verifiable from the one before, or none when
// target is not above it. The contract only accepts
// headers above the trusted height, so a target a concurrent update left
// behind cannot be installed from the latest state; it fails here unless the
// client already stores it, since the packet proofs at target would revert.
func (g *Generator) ClientUpdatePayloads(ctx context.Context, target uint64) ([][]byte, error) {
	state, err := g.host.BesuClientState(ctx, g.clientID)
	if err != nil {
		return nil, err
	}

	trustedHeight := state.LatestHeight.RevisionHeight
	if target == trustedHeight {
		return nil, nil
	}
	if target < trustedHeight {
		if _, errStored := g.host.BesuConsensusStateHash(ctx, g.clientID, target); errStored != nil {
			return nil, fmt.Errorf(
				"client advanced to height %d past target %d, which it does not store; retry at a newer height: %w",
				trustedHeight, target, errStored,
			)
		}
		return nil, nil
	}

	trusted, err := g.trustedHeader(ctx, trustedHeight)
	if err != nil {
		return nil, err
	}
	targetHeader, err := g.sealedHeader(ctx, target)
	if err != nil {
		return nil, err
	}

	headers, stuck, err := g.path(ctx, trusted, targetHeader, state.TrustLevel)
	if err != nil {
		return nil, err
	}
	if stuck != nil {
		return nil, stuck
	}

	updates := make([][]byte, len(headers))
	for i, header := range headers {
		updates[i], err = besu.EncodeUpdateClient(besumsgs.IBesuLightClientMsgsMsgUpdateClient{
			HeaderRlp:              header.RLP,
			TrustedHeight:          besumsgs.IICS02ClientMsgsHeight{RevisionHeight: trusted.Height},
			ConsensusStatePreimage: trusted.ConsensusState(),
		})
		if err != nil {
			return nil, fmt.Errorf("encoding update to height %d: %w", header.Height, err)
		}
		trusted = header
	}
	return updates, nil
}

func (g *Generator) trustedHeader(ctx context.Context, height uint64) (*besu.ParsedHeader, error) {
	if header := g.trusted.Load(); header != nil && header.Height == height {
		return header, nil
	}
	header, err := g.counterparty.SealedHeader(ctx, height)
	if err != nil {
		return nil, fmt.Errorf("reading trusted header: %w", err)
	}
	g.trusted.Store(header)
	return header, nil
}

// PacketProofs proves each packet's claim against the router storage at
// height with one eth_getProof call. Only the first proof carries the router
// account proof: the contract caches the proven storage root for the rest of
// the transaction.
func (g *Generator) PacketProofs(
	ctx context.Context,
	height uint64,
	kind v2.ProofKind,
	packets []channeltypesv2.Packet,
	_ []channeltypesv2.Acknowledgement,
) ([][]byte, error) {
	slots := make([]common.Hash, len(packets))
	for i, packet := range packets {
		path, err := kind.CommitmentPath(packet)
		if err != nil {
			return nil, err
		}
		slots[i] = besu.CommitmentSlot(path)
	}

	header, err := g.counterparty.SealedHeader(ctx, height)
	if err != nil {
		return nil, err
	}

	routerProof, err := g.counterparty.GetRouterProof(ctx, height, slots)
	if err != nil {
		return nil, fmt.Errorf("proving router at height %d: %w", height, err)
	}

	consensus := header.ConsensusState()
	proofs := make([][]byte, len(packets))
	for i, packet := range packets {
		accountProof := routerProof.AccountProof
		if i > 0 {
			accountProof = nil
		}
		proof, err := besu.EncodeMembershipProof(besumsgs.IBesuLightClientMsgsMembershipProof{
			ConsensusStatePreimage: consensus,
			AccountProofNodes:      accountProof,
			ProofNodes:             routerProof.StorageProofs[i],
		})
		if err != nil {
			return nil, fmt.Errorf("packet sequence %d: %w", packet.Sequence, err)
		}
		proofs[i] = proof
	}
	return proofs, nil
}
