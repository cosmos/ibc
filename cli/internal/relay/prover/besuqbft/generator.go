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
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	hostv2 "github.com/cosmos/ibc-go/v11/modules/core/24-host/v2"
	"github.com/cosmos/ibc/cli/besu"
	"github.com/cosmos/ibc/cli/internal/chains"
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

// Generator implements prover.Prover for one Besu QBFT light client. host is
// the chain the client lives on; counterparty is the Besu chain it tracks.
type Generator struct {
	host         chains.Client
	counterparty chains.Client
	clientID     string

	mu    sync.Mutex
	cache map[uint64]cacheEntry
}

// cacheEntry is a consensus state the prover derived from the counterparty.
// verified means its hash was checked against the light client's storage, so
// it may be used as a trusted anchor without another chain read.
type cacheEntry struct {
	state    besu.ConsensusState
	verified bool
}

// snapshot is everything the prover reads about one counterparty height.
type snapshot struct {
	header    *besu.Header
	proof     v2.AccountProof
	consensus besu.ConsensusState
}

// New builds a Generator without touching either chain; ResolveGenerator is
// the production entry point.
func New(host, counterparty chains.Client, clientID string) *Generator {
	return &Generator{
		host:         host,
		counterparty: counterparty,
		clientID:     clientID,
		cache:        make(map[uint64]cacheEntry),
	}
}

// ResolveGenerator builds the prover for self, tracking counterparty, and
// fails fast unless self's registered light client is a Besu QBFT client whose
// tracked router is counterparty's configured router.
func ResolveGenerator(
	ctx context.Context,
	self, counterparty config.ClientEnd,
	counterpartyRouter string,
	clientSet *chains.ClientSet,
) (*Generator, error) {
	host, ok := clientSet.Get(self.ChainID)
	if !ok {
		return nil, fmt.Errorf("client %q: no configured chain client for %q", self.ClientID, self.ChainID)
	}

	counterpartyChain, ok := clientSet.Get(counterparty.ChainID)
	if !ok {
		return nil, fmt.Errorf(
			"client %q: no configured chain client for counterparty chain %q", self.ClientID, counterparty.ChainID,
		)
	}

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
		return fmt.Errorf("client %q: warming trusted consensus state: %w", g.clientID, err)
	}

	return nil
}

// LatestProvableHeight returns the newest counterparty height the light client
// will accept now: every QBFT block is final, so this is the chain head unless
// its timestamp exceeds the host chain's time plus the client's clock drift
// allowance, in which case it steps back to the newest admissible header.
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
		return 0, time.Time{}, expiredErr
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

// StateProof returns the client updates that bring the light client to
// target, one per updateClient call in submission order. Above the trusted
// height each update is the furthest header the previous one's validators
// accept, so the sequence is empty only when the client already stores target.
func (g *Generator) StateProof(ctx context.Context, target uint64) ([][]byte, error) {
	state, err := g.host.GetBesuQBFTClientState(ctx, g.clientID)
	if err != nil {
		return nil, err
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
	targetHeader, err := g.header(ctx, target)
	if err != nil {
		return nil, err
	}
	maxTimestamp := hostHead.Timestamp.Add(time.Duration(state.MaxClockDrift) * time.Second)
	if time.Unix(int64(targetHeader.Timestamp), 0).After(maxTimestamp) {
		return nil, fmt.Errorf("target height %d exceeds host clock drift", target)
	}

	if target <= state.LatestHeight {
		snap, err := g.snapshotForHeader(ctx, targetHeader, nil)
		if err != nil {
			return nil, err
		}
		update, err := g.updateAtOrBelowTrusted(ctx, state.LatestHeight, trusted, snap)
		if err != nil || update == nil {
			return nil, err
		}
		return [][]byte{update}, nil
	}

	var updates [][]byte
	trustedHeight := state.LatestHeight
	for {
		header, err := g.nextHeader(ctx, trustedHeight, trusted, targetHeader)
		if err != nil {
			return nil, err
		}
		snap, err := g.snapshotForHeader(ctx, header, nil)
		if err != nil {
			return nil, err
		}
		update, err := besu.EncodeUpdateClient(header.RLP, trustedHeight, trusted, snap.proof.AccountProof)
		if err != nil {
			return nil, fmt.Errorf("encoding update to height %d: %w", header.Height, err)
		}
		updates = append(updates, update)
		if header.Height == target {
			g.store(target, snap.consensus, false)
			return updates, nil
		}
		trustedHeight, trusted = header.Height, snap.consensus
	}
}

// PacketProofs proves each packet's claim against the router storage at
// height, sharing one eth_getProof call across packets with the same slot.
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

// updateAtOrBelowTrusted handles a target the client may already store: no
// update when the stored hash matches, an error when it conflicts, and a
// single backfill update from the trusted height when nothing is stored.
func (g *Generator) updateAtOrBelowTrusted(
	ctx context.Context,
	trustedHeight uint64,
	trusted besu.ConsensusState,
	snap *snapshot,
) ([]byte, error) {
	target := snap.header.Height
	var err error

	var stored [32]byte
	if target == trustedHeight {
		stored, err = trusted.Hash()
	} else {
		stored, err = g.host.GetBesuQBFTConsensusStateHash(ctx, g.clientID, target)
	}

	switch {
	case err == nil:
		hash, hashErr := snap.consensus.Hash()
		if hashErr != nil {
			return nil, hashErr
		}

		if hash != common.Hash(stored) {
			return nil, fmt.Errorf("%w: height %d stores %s, counterparty state hashes to %s",
				ErrConflictingConsensusState, target, common.Hash(stored), hash)
		}

		return nil, nil
	case !errors.Is(err, v2.ErrConsensusStateNotFound):
		return nil, err
	}

	signers, err := snap.header.Signers()
	if err != nil {
		return nil, fmt.Errorf("header %d: %w", target, err)
	}

	if checkErr := besu.CheckUpdate(snap.header, signers, trusted); checkErr != nil {
		return nil, fmt.Errorf("backfilling height %d from trusted height %d: %w", target, trustedHeight, checkErr)
	}

	update, err := besu.EncodeUpdateClient(snap.header.RLP, trustedHeight, trusted, snap.proof.AccountProof)
	if err != nil {
		return nil, fmt.Errorf("encoding update to height %d: %w", target, err)
	}

	return update, nil
}

// nextHeader returns the newest header at or below target that trusted
// accepts: target itself when possible, otherwise the result of bisecting the
// heights in between. Acceptance is monotone while validators leave and do not
// return; when it is not, the probe still only advances on accepted headers
// and its last candidate is trustedHeight+1, so it fails only when no
// checkpoint exists there either.
func (g *Generator) nextHeader(
	ctx context.Context,
	trustedHeight uint64,
	trusted besu.ConsensusState,
	target *besu.Header,
) (*besu.Header, error) {
	check := func(header *besu.Header) (error, error) {
		signers, err := header.Signers()
		if err != nil {
			return nil, fmt.Errorf("header %d: %w", header.Height, err)
		}
		return besu.CheckUpdate(header, signers, trusted), nil
	}
	rejected, err := check(target)
	if err != nil {
		return nil, err
	}
	if rejected == nil {
		return target, nil
	}
	var last *besu.Header
	rejectedAt := target.Height
	low, high := trustedHeight, target.Height-1
	for low < high {
		mid := low + (high-low+1)/2
		header, err := g.header(ctx, mid)
		if err != nil {
			return nil, err
		}
		checkErr, err := check(header)
		if err != nil {
			return nil, err
		}
		if checkErr != nil {
			rejected, rejectedAt = checkErr, mid
			high = mid - 1
			continue
		}
		low, last = mid, header
	}
	if last == nil {
		return nil, fmt.Errorf(
			"no header after trusted height %d is accepted (height %d: %w)", trustedHeight, rejectedAt, rejected,
		)
	}
	return last, nil
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
		proof, err := besu.EncodeMembershipProof(snap.consensus, storage.Proof)
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
	raw, err := g.counterparty.GetHeaderRLP(ctx, height)
	if err != nil {
		return nil, fmt.Errorf("reading counterparty header %d: %w", height, err)
	}

	header, err := besu.ParseHeader(raw)
	if err != nil {
		return nil, fmt.Errorf("counterparty header %d: %w", height, err)
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

	return g.snapshotForHeader(ctx, header, slots)
}

func (g *Generator) snapshotForHeader(ctx context.Context, header *besu.Header, slots [][32]byte) (*snapshot, error) {
	height := header.Height
	proof, err := g.counterparty.GetRouterProof(ctx, height, slots)
	if err != nil {
		return nil, fmt.Errorf("proving router at height %d: %w (%s)", height, err, historyHint)
	}

	return &snapshot{
		header: header,
		proof:  proof,
		consensus: besu.ConsensusState{
			Timestamp:   header.Timestamp,
			StorageRoot: proof.StorageRoot,
			Validators:  header.Validators,
		},
	}, nil
}

// preimage returns the consensus state the light client stores at height,
// verified against the stored hash the first time it is used as an anchor.
func (g *Generator) preimage(ctx context.Context, height uint64) (besu.ConsensusState, error) {
	g.mu.Lock()
	entry, ok := g.cache[height]
	g.mu.Unlock()

	if ok && entry.verified {
		return entry.state, nil
	}

	state := entry.state

	if !ok {
		snap, err := g.snapshot(ctx, height, nil)
		if err != nil {
			return besu.ConsensusState{}, fmt.Errorf("rebuilding trusted consensus state at height %d: %w", height, err)
		}

		state = snap.consensus
	}

	stored, err := g.host.GetBesuQBFTConsensusStateHash(ctx, g.clientID, height)
	if err != nil {
		return besu.ConsensusState{}, err
	}

	hash, err := state.Hash()
	if err != nil {
		return besu.ConsensusState{}, err
	}

	if hash != common.Hash(stored) {
		g.mu.Lock()
		if current, exists := g.cache[height]; exists && !current.verified {
			delete(g.cache, height)
		}
		g.mu.Unlock()

		return besu.ConsensusState{}, fmt.Errorf(
			"consensus state rebuilt for height %d hashes to %s but the light client stores %s",
			height, hash, common.Hash(stored),
		)
	}

	g.store(height, state, true)

	return state, nil
}

// store caches state for height, never replacing a verified entry. A verified
// entry is the client's latest height, which the contract never lowers, so
// everything below it can no longer be read and is dropped.
func (g *Generator) store(height uint64, state besu.ConsensusState, verified bool) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if existing, ok := g.cache[height]; ok && existing.verified {
		return
	}

	g.cache[height] = cacheEntry{state: state, verified: verified}
	if !verified {
		return
	}

	for h := range g.cache {
		if h < height {
			delete(g.cache, h)
		}
	}
}
