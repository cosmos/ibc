// SPDX-License-Identifier: Apache-2.0

// Package besuqbft implements prover.Prover for a Besu QBFT light client: it
// reads the client's state from the host chain, fetches sealed headers and
// eth_getProof results from the Besu chain it tracks and encodes update and
// membership payloads. The contract verifies everything they carry, checked
// by simulation at submission.
package besuqbft

import (
	"context"
	"fmt"
	"time"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besumsgs"
	"github.com/ethereum/go-ethereum/common"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	"github.com/cosmos/ibc/cli/besu"
	"github.com/cosmos/ibc/cli/internal/chains/evm"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

// Host is the chain the light client lives on. It needs no Besu consensus:
// any EVM chain hosting the contract qualifies.
type Host interface {
	BesuQBFTClientState(ctx context.Context, clientID string) (besumsgs.IBesuLightClientMsgsClientState, error)
}

// Counterparty is the Besu chain the light client tracks.
type Counterparty interface {
	ChainID() string
	RouterAddress() common.Address
	GetBlockHeader(ctx context.Context, height uint64) (v2.BlockHeader, error)
	// SealedHeader returns the parsed header at exactly height.
	SealedHeader(ctx context.Context, height uint64) (*besu.ParsedHeader, error)
	GetRouterProof(ctx context.Context, height uint64, slots [][32]byte) (evm.RouterProof, error)
}

// Generator implements prover.Prover for one Besu QBFT light client.
type Generator struct {
	host         Host
	counterparty Counterparty
	clientID     string
}

// NewGenerator builds the prover for clientID on host, tracking counterparty,
// and fails fast unless the client proves counterparty's configured router.
func NewGenerator(ctx context.Context, clientID string, host Host, counterparty Counterparty) (*Generator, error) {
	state, err := host.BesuQBFTClientState(ctx, clientID)
	if err != nil {
		return nil, err
	}

	if state.IbcRouter != counterparty.RouterAddress() {
		return nil, fmt.Errorf(
			"client %q proves router %s but chain %s is configured with router %s",
			clientID, state.IbcRouter, counterparty.ChainID(), counterparty.RouterAddress(),
		)
	}

	return &Generator{host: host, counterparty: counterparty, clientID: clientID}, nil
}

// LatestProvableHeight returns the counterparty head. The light client
// rejects an update it cannot verify when the relay is simulated.
func (g *Generator) LatestProvableHeight(ctx context.Context) (uint64, time.Time, error) {
	head, err := g.counterparty.GetBlockHeader(ctx, v2.LatestBlock)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("reading counterparty chain head: %w", err)
	}

	return head.Height, head.Timestamp, nil
}

// ClientUpdatePayloads returns one updateMsg from the client's latest
// consensus state to target, or none when target is the latest height. After
// full verification the contract installs a target below the latest height as
// a historical consensus state, or no-ops when it already stores it.
func (g *Generator) ClientUpdatePayloads(ctx context.Context, target uint64) ([][]byte, error) {
	state, err := g.host.BesuQBFTClientState(ctx, g.clientID)
	if err != nil {
		return nil, err
	}

	trustedHeight := state.LatestHeight.RevisionHeight
	if target == trustedHeight {
		return nil, nil
	}

	trusted, err := g.counterparty.SealedHeader(ctx, trustedHeight)
	if err != nil {
		return nil, fmt.Errorf("reading trusted header: %w", err)
	}

	targetHeader, err := g.counterparty.SealedHeader(ctx, target)
	if err != nil {
		return nil, err
	}

	update, err := besu.EncodeUpdateClient(besumsgs.IBesuLightClientMsgsMsgUpdateClient{
		HeaderRlp:              targetHeader.RLP,
		TrustedHeight:          besumsgs.IICS02ClientMsgsHeight{RevisionHeight: trustedHeight},
		ConsensusStatePreimage: besu.ConsensusStateOf(trusted),
	})
	if err != nil {
		return nil, fmt.Errorf("encoding update to height %d: %w", target, err)
	}
	return [][]byte{update}, nil
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
) ([][]byte, error) {
	slots := make([][32]byte, len(packets))
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

	consensus := besu.ConsensusStateOf(header)
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
