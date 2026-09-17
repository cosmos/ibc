// SPDX-License-Identifier: Apache-2.0

// Package besuqbft implements prover.Prover for a Besu QBFT light client: it
// reads the client's trusted state from the host chain, fetches sealed headers
// and eth_getProof results from the Besu chain it tracks, checks the client's
// threshold rules off-chain and encodes update and membership payloads.
package besuqbft

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	hostv2 "github.com/cosmos/ibc-go/v11/modules/core/24-host/v2"
	"github.com/cosmos/ibc/cli/besu"
	"github.com/cosmos/ibc/cli/internal/chains/evm"
	"github.com/cosmos/ibc/cli/internal/config"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

// Errors the prover surfaces to the pipeline.
var (
	ErrClientExpired = errors.New(
		"besu qbft light client trusting period has expired; the client must be redeployed",
	)
	ErrConflictingConsensusState = errors.New(
		"besu qbft light client stores a different consensus state at the target height",
	)
	ErrCommitmentMismatch = errors.New("packet commitment on the counterparty does not match the packet")
	ErrAckMissing         = errors.New("no acknowledgement commitment on the counterparty")
	ErrReceiptExists      = errors.New("packet receipt exists on the counterparty")
)

const historyHint = "the counterparty node may not serve state this old: raise its Bonsai history limit or use an archive node"

// chain is the EVM reads the prover needs. *evm.Client is the production
// implementation; tests use a fake.
type chain interface {
	ChainID() string
	GetBlockHeader(ctx context.Context, height uint64) (v2.BlockHeader, error)
	SealedHeader(ctx context.Context, height uint64) (*besu.Header, error)
	GetRouterProof(ctx context.Context, height uint64, slots [][32]byte) (evm.AccountProof, error)
	GetBesuQBFTClientState(ctx context.Context, clientID string) (besu.ClientState, error)
	GetBesuQBFTConsensusStateHash(ctx context.Context, clientID string, height uint64) ([32]byte, error)
}

// Generator implements prover.Prover for one Besu QBFT light client. host is
// the chain the client lives on; counterparty is the Besu chain it tracks.
type Generator struct {
	host         chain
	counterparty chain
	clientID     string
}

// snapshot is everything the prover reads about one counterparty height for
// packet proofs: the header and the router's account and storage proofs.
type snapshot struct {
	header    *besu.Header
	proof     evm.AccountProof
	consensus besu.ConsensusState
}

// consensusOf is the consensus state an update to header installs. Every
// field comes from the header, so no historical state is needed.
func consensusOf(header *besu.Header) besu.ConsensusState {
	return besu.ConsensusState{Timestamp: header.Timestamp, StateRoot: header.StateRoot, Validators: header.Validators}
}

// New builds a Generator without touching either chain; ResolveGenerator is
// the production entry point.
func New(host, counterparty chain, clientID string) *Generator {
	return &Generator{host: host, counterparty: counterparty, clientID: clientID}
}

// ResolveGenerator builds the prover for self, tracking counterparty, and
// fails fast unless self's registered light client is a Besu QBFT client whose
// tracked router is counterparty's configured router.
func ResolveGenerator(
	ctx context.Context,
	self config.ClientEnd,
	counterpartyRouter string,
	host, counterpartyChain *evm.Client,
) (*Generator, error) {
	gen := New(host, counterpartyChain, self.ClientID)
	if err := gen.resolve(ctx, counterpartyRouter); err != nil {
		return nil, err
	}

	return gen, nil
}

func (g *Generator) resolve(ctx context.Context, counterpartyRouter string) error {
	state, err := g.host.GetBesuQBFTClientState(ctx, g.clientID)
	if err != nil {
		return fmt.Errorf("client %q is not a besu-qbft light client: %w", g.clientID, err)
	}

	if !common.IsHexAddress(counterpartyRouter) {
		return fmt.Errorf("client %q: invalid counterparty router address %q", g.clientID, counterpartyRouter)
	}

	if state.IBCRouter != common.HexToAddress(counterpartyRouter) {
		return fmt.Errorf(
			"client %q proves router %s but chain %s is configured with router %s",
			g.clientID, state.IBCRouter, g.counterparty.ChainID(), counterpartyRouter,
		)
	}

	if _, err := g.preimage(ctx, state.LatestHeight); err != nil {
		return fmt.Errorf("client %q: verifying trusted consensus state: %w", g.clientID, err)
	}

	return nil
}

// LatestProvableHeight returns the newest counterparty height within clock drift:
// every QBFT block is final, so this is the chain head unless
// its timestamp exceeds the host chain's time plus the client's clock drift
// allowance, in which case it steps back to the newest admissible header.
// An expired client can still prove packets at its latest trusted height.
func (g *Generator) LatestProvableHeight(ctx context.Context) (uint64, time.Time, error) {
	state, err := g.host.GetBesuQBFTClientState(ctx, g.clientID)
	if err != nil {
		return 0, time.Time{}, err
	}

	trusted, err := g.preimage(ctx, state.LatestHeight)
	if err != nil {
		return 0, time.Time{}, err
	}

	hostHead, err := g.host.GetBlockHeader(ctx, v2.LatestBlock)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("reading host chain head: %w", err)
	}
	if expiredErr := checkTrustingPeriod(state, trusted, hostHead.Timestamp); expiredErr != nil {
		return state.LatestHeight, time.Unix(int64(trusted.Timestamp), 0).UTC(), nil
	}

	head, err := g.counterparty.GetBlockHeader(ctx, v2.LatestBlock)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("reading counterparty chain head: %w", err)
	}

	maxTimestamp := hostHead.Timestamp.Add(time.Duration(state.MaxClockDrift) * time.Second) //nolint:gosec // seconds
	height, timestamp := head.Height, head.Timestamp

	if timestamp.After(maxTimestamp) {
		low, high := state.LatestHeight, height
		timestamp = time.Unix(int64(trusted.Timestamp), 0).UTC()
		for low < high {
			mid := low + (high-low)/2 + 1
			header, err := g.counterparty.GetBlockHeader(ctx, mid)
			if err != nil {
				return 0, time.Time{}, fmt.Errorf("reading counterparty header %d: %w", mid, err)
			}
			if header.Timestamp.After(maxTimestamp) {
				high = mid - 1
			} else {
				low = mid
				timestamp = header.Timestamp
			}
		}
		height = low
	}

	return height, timestamp, nil
}

func checkTrustingPeriod(state besu.ClientState, trusted besu.ConsensusState, hostTime time.Time) error {
	if state.TrustingPeriod == 0 {
		return nil
	}

	// the contract requires trusted.timestamp + trustingPeriod > block.timestamp
	if trusted.Timestamp+state.TrustingPeriod <= uint64(hostTime.Unix()) { //nolint:gosec // seconds since epoch
		return fmt.Errorf(
			"%w: trusted height %d timestamp %d, trusting period %ds",
			ErrClientExpired, state.LatestHeight, trusted.Timestamp, state.TrustingPeriod,
		)
	}

	return nil
}

// ClientUpdatePayload returns an encoded updateMsg from the client's trusted state to target,
// or nil when the client already stores target. Intermediate updates are not supported.
func (g *Generator) ClientUpdatePayload(ctx context.Context, target uint64) ([]byte, error) {
	state, err := g.host.GetBesuQBFTClientState(ctx, g.clientID)
	if err != nil {
		return nil, err
	}
	targetHeader, err := g.header(ctx, target)
	if err != nil {
		return nil, err
	}
	if target <= state.LatestHeight {
		stored, err := g.host.GetBesuQBFTConsensusStateHash(ctx, g.clientID, target)
		switch {
		case err == nil:
			hash, err := consensusOf(targetHeader).Hash()
			if err != nil {
				return nil, err
			}
			if hash != common.Hash(stored) {
				return nil, fmt.Errorf("%w: height %d stores %s, counterparty state hashes to %s",
					ErrConflictingConsensusState, target, common.Hash(stored), hash)
			}
			return nil, nil
		case !errors.Is(err, evm.ErrConsensusStateNotFound):
			return nil, err
		}
	}
	trusted, err := g.preimage(ctx, state.LatestHeight)
	if err != nil {
		return nil, err
	}
	hostHead, err := g.host.GetBlockHeader(ctx, v2.LatestBlock)
	if err != nil {
		return nil, fmt.Errorf("reading host chain head: %w", err)
	}
	if trustErr := checkTrustingPeriod(state, trusted, hostHead.Timestamp); trustErr != nil {
		return nil, trustErr
	}
	maxTimestamp := hostHead.Timestamp.Add(time.Duration(state.MaxClockDrift) * time.Second)
	if time.Unix(int64(targetHeader.Timestamp), 0).After(maxTimestamp) {
		return nil, fmt.Errorf("target height %d exceeds host clock drift", target)
	}

	signers, err := targetHeader.Signers()
	if err != nil {
		return nil, fmt.Errorf("header %d: %w", target, err)
	}
	if checkErr := besu.CheckUpdate(targetHeader, signers, trusted); checkErr != nil {
		return nil, fmt.Errorf(
			"direct update from trusted height %d to %d failed (intermediate updates are not supported): %w",
			state.LatestHeight, target, checkErr,
		)
	}
	update, err := besu.EncodeUpdateClient(targetHeader.RLP, state.LatestHeight, trusted)
	if err != nil {
		return nil, fmt.Errorf("encoding update to height %d: %w", target, err)
	}
	return update, nil
}

// PacketProofs proves each packet's claim against the router storage at
// height, sharing one eth_getProof call across packets with the same slot.
// Only the first proof carries the router account proof: the contract caches
// the proven storage root for the rest of the transaction.
func (g *Generator) PacketProofs(
	ctx context.Context,
	height uint64,
	kind v2.ProofKind,
	packets []channeltypesv2.Packet,
) ([][]byte, error) {
	slots, indices, err := packetSlots(kind, packets)
	if err != nil {
		return nil, err
	}
	snap, err := g.snapshot(ctx, height, slots)
	if err != nil {
		return nil, err
	}
	return packetProofs(snap, kind, packets, indices)
}

func packetSlots(kind v2.ProofKind, packets []channeltypesv2.Packet) ([][32]byte, []int, error) {
	unique := make([][32]byte, 0, len(packets))
	index := make(map[[32]byte]int, len(packets))
	indices := make([]int, len(packets))
	for i, packet := range packets {
		path, err := packetPath(kind, packet)
		if err != nil {
			return nil, nil, err
		}
		slot := besu.CommitmentSlot(path)
		idx, seen := index[slot]
		if !seen {
			idx = len(unique)
			index[slot] = idx
			unique = append(unique, slot)
		}
		indices[i] = idx
	}
	if len(unique) == 0 {
		unique = nil
	}
	return unique, indices, nil
}

func packetProofs(snap *snapshot, kind v2.ProofKind, packets []channeltypesv2.Packet, indices []int) ([][]byte, error) {
	proofs := make([][]byte, len(packets))
	for i, packet := range packets {
		storage := snap.proof.StorageProofs[indices[i]]
		if err := checkValue(kind, packet, common.BigToHash(storage.Value)); err != nil {
			return nil, fmt.Errorf("packet sequence %d at height %d: %w", packet.Sequence, snap.header.Height, err)
		}
		accountProof := snap.proof.AccountProof
		if i > 0 {
			accountProof = nil
		}
		proof, err := besu.EncodeMembershipProof(snap.consensus, accountProof, storage.Proof)
		if err != nil {
			return nil, fmt.Errorf("packet sequence %d: %w", packet.Sequence, err)
		}
		proofs[i] = proof
	}
	return proofs, nil
}

// packetPath is the raw commitment path for kind, keyed by the client the
// counterparty router stores it under.
func packetPath(kind v2.ProofKind, packet channeltypesv2.Packet) ([]byte, error) {
	switch kind {
	case v2.ProofKindPacketCommitment:
		return hostv2.PacketCommitmentKey(packet.SourceClient, packet.Sequence), nil
	case v2.ProofKindAcknowledgement:
		return hostv2.PacketAcknowledgementKey(packet.DestinationClient, packet.Sequence), nil
	case v2.ProofKindReceiptAbsence:
		return hostv2.PacketReceiptKey(packet.DestinationClient, packet.Sequence), nil
	default:
		return nil, fmt.Errorf("unsupported proof kind %v", kind)
	}
}

// checkValue verifies the slot value matches the claim before it is proven.
func checkValue(kind v2.ProofKind, packet channeltypesv2.Packet, value common.Hash) error {
	switch kind {
	case v2.ProofKindPacketCommitment:
		expected := common.BytesToHash(channeltypesv2.CommitPacket(packet))
		if value != expected {
			return fmt.Errorf("%w: stored %s, packet commits to %s", ErrCommitmentMismatch, value, expected)
		}
	case v2.ProofKindAcknowledgement:
		if value == (common.Hash{}) {
			return ErrAckMissing
		}
	case v2.ProofKindReceiptAbsence:
		if value != (common.Hash{}) {
			return ErrReceiptExists
		}
	default:
		return fmt.Errorf("unsupported proof kind %v", kind)
	}

	return nil
}

// header fetches and parses the counterparty header at height.
func (g *Generator) header(ctx context.Context, height uint64) (*besu.Header, error) {
	header, err := g.counterparty.SealedHeader(ctx, height)
	if err != nil {
		return nil, fmt.Errorf("reading counterparty header %d: %w", height, err)
	}

	if header.Height != height {
		return nil, fmt.Errorf("counterparty returned header %d for height %d", header.Height, height)
	}

	return header, nil
}

// snapshot reads the header and router proof at height and derives
// the consensus state an update to that height installs.
func (g *Generator) snapshot(ctx context.Context, height uint64, slots [][32]byte) (*snapshot, error) {
	header, err := g.header(ctx, height)
	if err != nil {
		return nil, err
	}

	proof, err := g.counterparty.GetRouterProof(ctx, height, slots)
	if err != nil {
		return nil, fmt.Errorf("proving router at height %d: %w (%s)", height, err, historyHint)
	}

	return &snapshot{header: header, proof: proof, consensus: consensusOf(header)}, nil
}

// preimage returns the consensus state the light client stores at height,
// rebuilt from the counterparty header and checked against the stored hash.
func (g *Generator) preimage(ctx context.Context, height uint64) (besu.ConsensusState, error) {
	header, err := g.header(ctx, height)
	if err != nil {
		return besu.ConsensusState{}, fmt.Errorf("rebuilding trusted consensus state at height %d: %w", height, err)
	}

	state := consensusOf(header)
	stored, err := g.host.GetBesuQBFTConsensusStateHash(ctx, g.clientID, height)
	if err != nil {
		return besu.ConsensusState{}, err
	}

	hash, err := state.Hash()
	if err != nil {
		return besu.ConsensusState{}, err
	}

	if hash != common.Hash(stored) {
		return besu.ConsensusState{}, fmt.Errorf(
			"consensus state rebuilt for height %d hashes to %s but the light client stores %s",
			height, hash, common.Hash(stored),
		)
	}

	return state, nil
}
