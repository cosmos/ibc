// SPDX-License-Identifier: Apache-2.0

// Package besuqbft implements prover.Prover for a Besu QBFT light client: it
// reads the client's trusted state from the host chain, fetches sealed headers
// and eth_getProof results from the Besu chain it tracks and encodes update
// and membership payloads. Contract simulation at submission verifies consensus.
package besuqbft

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besumsgs"
	"github.com/ethereum/go-ethereum/common"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	hostv2 "github.com/cosmos/ibc-go/v11/modules/core/24-host/v2"
	"github.com/cosmos/ibc/cli/besu"
	chainsbesu "github.com/cosmos/ibc/cli/internal/chains/besu"
	"github.com/cosmos/ibc/cli/internal/chains/evm"
	"github.com/cosmos/ibc/cli/internal/config"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

// Errors the prover surfaces to the pipeline.
var (
	ErrClientExpired = errors.New(
		"besu qbft light client trusting period has expired; the consensus state is no longer usable",
	)
	ErrConflictingConsensusState = errors.New(
		"besu qbft light client stores a different consensus state at the target height",
	)
	ErrCommitmentMismatch = errors.New("packet commitment on the counterparty does not match the packet")
	ErrAckMissing         = errors.New("no acknowledgement commitment on the counterparty")
	ErrReceiptExists      = errors.New("packet receipt exists on the counterparty")
)

const historyHint = "the counterparty node may not serve state this old: raise its Bonsai history limit or use an archive node"

// Host is the chain the light client lives on. It needs no Besu consensus:
// any EVM chain hosting the contract qualifies.
type Host interface {
	GetBlockHeader(ctx context.Context, height uint64) (v2.BlockHeader, error)
	ClientState(ctx context.Context, clientID string) (besumsgs.IBesuLightClientMsgsClientState, error)
	ConsensusStateHash(ctx context.Context, clientID string, height uint64) ([32]byte, error)
}

// Counterparty is the Besu chain the light client tracks.
type Counterparty interface {
	ChainID() string
	GetBlockHeader(ctx context.Context, height uint64) (v2.BlockHeader, error)
	SealedHeader(ctx context.Context, height uint64) (*besu.Header, error)
	GetRouterProof(ctx context.Context, height uint64, slots [][32]byte) (evm.AccountProof, error)
}

// Generator implements prover.Prover for one Besu QBFT light client.
type Generator struct {
	host         Host
	counterparty Counterparty
	clientID     string
}

// snapshot is everything the prover reads about one counterparty height for
// packet proofs: the header and the router's account and storage proofs.
type snapshot struct {
	header    *besu.Header
	proof     evm.AccountProof
	consensus besumsgs.IBesuLightClientMsgsConsensusState
}

// consensusOf is the consensus state an update to header installs. Every
// field comes from the header, so no historical state is needed.
func consensusOf(header *besu.Header) besumsgs.IBesuLightClientMsgsConsensusState {
	return besumsgs.IBesuLightClientMsgsConsensusState{
		Timestamp:  header.Timestamp,
		StateRoot:  header.StateRoot,
		Validators: header.Validators,
	}
}

// New builds a Generator without touching either chain; ResolveGenerator is
// the production entry point.
func New(host Host, counterparty Counterparty, clientID string) *Generator {
	return &Generator{host: host, counterparty: counterparty, clientID: clientID}
}

// ResolveGenerator builds the prover for self, tracking counterparty, and
// fails fast unless self's registered light client is a Besu QBFT client whose
// tracked router is counterparty's configured router.
func ResolveGenerator(
	ctx context.Context,
	self config.ClientEnd,
	counterpartyRouter string,
	host Host,
	counterpartyChain Counterparty,
) (*Generator, error) {
	gen := New(host, counterpartyChain, self.ClientID)
	if err := gen.resolve(ctx, counterpartyRouter); err != nil {
		return nil, err
	}

	return gen, nil
}

func unixSeconds(timestamp time.Time) (uint64, error) {
	seconds := timestamp.Unix()
	if seconds < 0 {
		return 0, errors.New("negative chain timestamp")
	}
	return uint64(seconds), nil
}

func exceedsClockDrift(target, host, drift uint64) bool {
	return target > host && target-host > drift
}

func (g *Generator) resolve(ctx context.Context, counterpartyRouter string) error {
	state, err := g.host.ClientState(ctx, g.clientID)
	if err != nil {
		return fmt.Errorf("client %q is not a besu-qbft light client: %w", g.clientID, err)
	}

	if !common.IsHexAddress(counterpartyRouter) {
		return fmt.Errorf("client %q: invalid counterparty router address %q", g.clientID, counterpartyRouter)
	}

	if state.IbcRouter != common.HexToAddress(counterpartyRouter) {
		return fmt.Errorf(
			"client %q proves router %s but chain %s is configured with router %s",
			g.clientID, state.IbcRouter, g.counterparty.ChainID(), counterpartyRouter,
		)
	}

	if _, err := g.preimage(ctx, state.LatestHeight.RevisionHeight); err != nil {
		return fmt.Errorf("client %q: verifying trusted consensus state: %w", g.clientID, err)
	}

	return nil
}

// LatestProvableHeight selects the newest clock-admissible height. Consensus
// validity is left to contract simulation; the relayer never chains several
// updates to bridge validator turnover.
func (g *Generator) LatestProvableHeight(ctx context.Context) (uint64, time.Time, error) {
	state, err := g.host.ClientState(ctx, g.clientID)
	if err != nil {
		return 0, time.Time{}, err
	}

	trusted, err := g.preimage(ctx, state.LatestHeight.RevisionHeight)
	if err != nil {
		return 0, time.Time{}, err
	}

	hostHead, err := g.host.GetBlockHeader(ctx, v2.LatestBlock)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("reading host chain head: %w", err)
	}
	hostSeconds, err := unixSeconds(hostHead.Timestamp)
	if err != nil {
		return 0, time.Time{}, err
	}
	if expiredErr := checkTrustingPeriod(state.TrustingPeriod, trusted.Timestamp, hostSeconds); expiredErr != nil {
		return 0, time.Time{}, expiredErr
	}

	head, err := g.counterparty.GetBlockHeader(ctx, v2.LatestBlock)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("reading counterparty chain head: %w", err)
	}

	headSeconds, err := unixSeconds(head.Timestamp)
	if err != nil {
		return 0, time.Time{}, err
	}
	height, timestamp := head.Height, head.Timestamp

	if exceedsClockDrift(headSeconds, hostSeconds, state.MaxClockDrift) {
		low, high := state.LatestHeight.RevisionHeight, height
		timestamp = time.Unix(int64(trusted.Timestamp), 0).UTC()
		for low < high {
			mid := low + (high-low)/2 + 1
			header, err := g.counterparty.GetBlockHeader(ctx, mid)
			if err != nil {
				return 0, time.Time{}, fmt.Errorf("reading counterparty header %d: %w", mid, err)
			}
			seconds, err := unixSeconds(header.Timestamp)
			if err != nil {
				return 0, time.Time{}, err
			}
			if exceedsClockDrift(seconds, hostSeconds, state.MaxClockDrift) {
				high = mid - 1
			} else {
				low = mid
				timestamp = header.Timestamp
			}
		}
		height = low
	}

	if height != state.LatestHeight.RevisionHeight {
		header, headerErr := g.header(ctx, height)
		if headerErr != nil {
			return 0, time.Time{}, headerErr
		}
		if checkErr := checkUpdateTime(header, state, hostSeconds); checkErr != nil {
			return 0, time.Time{}, checkErr
		}
	}
	return height, timestamp, nil
}

func checkTrustingPeriod(period, timestamp, host uint64) error {
	// Equivalent to Solidity's widened timestamp + period > block.timestamp.
	if host >= timestamp && host-timestamp >= period {
		return fmt.Errorf("%w: timestamp %d, trusting period %ds", ErrClientExpired, timestamp, period)
	}
	return nil
}

// ClientUpdatePayload returns an encoded updateMsg from the client's latest
// trusted state to target, or nil when the client already stores an unexpired
// target. A target below the latest height that is not stored is installed as
// a historical consensus state, anchored on the latest one like any other
// update: the update is always one step, never a chain.
func (g *Generator) ClientUpdatePayload(ctx context.Context, target uint64) ([]byte, error) {
	state, err := g.host.ClientState(ctx, g.clientID)
	if err != nil {
		return nil, err
	}
	hostHead, err := g.host.GetBlockHeader(ctx, v2.LatestBlock)
	if err != nil {
		return nil, fmt.Errorf("reading host chain head: %w", err)
	}
	hostSeconds, err := unixSeconds(hostHead.Timestamp)
	if err != nil {
		return nil, err
	}
	targetHeader, err := g.header(ctx, target)
	if err != nil {
		return nil, err
	}
	if target <= state.LatestHeight.RevisionHeight {
		stored, storedErr := g.host.ConsensusStateHash(ctx, g.clientID, target)
		switch {
		case storedErr == nil:
			if trustErr := checkTrustingPeriod(
				state.TrustingPeriod,
				targetHeader.Timestamp,
				hostSeconds,
			); trustErr != nil {
				return nil, fmt.Errorf("proof height %d: %w", target, trustErr)
			}
			hash, hashErr := besu.HashConsensusState(consensusOf(targetHeader))
			if hashErr != nil {
				return nil, hashErr
			}
			if hash != common.Hash(stored) {
				return nil, fmt.Errorf("%w: height %d stores %s, counterparty state hashes to %s",
					ErrConflictingConsensusState, target, common.Hash(stored), hash)
			}
			return nil, nil
		case !errors.Is(storedErr, chainsbesu.ErrConsensusStateNotFound):
			return nil, storedErr
		}
	}
	trusted, err := g.preimage(ctx, state.LatestHeight.RevisionHeight)
	if err != nil {
		return nil, err
	}
	if trustErr := checkTrustingPeriod(state.TrustingPeriod, trusted.Timestamp, hostSeconds); trustErr != nil {
		return nil, trustErr
	}
	if checkErr := checkUpdateTime(targetHeader, state, hostSeconds); checkErr != nil {
		return nil, checkErr
	}
	update, err := besu.EncodeUpdateClient(besumsgs.IBesuLightClientMsgsMsgUpdateClient{
		HeaderRlp:              targetHeader.RLP,
		TrustedHeight:          besumsgs.IICS02ClientMsgsHeight{RevisionHeight: state.LatestHeight.RevisionHeight},
		ConsensusStatePreimage: trusted,
	})
	if err != nil {
		return nil, fmt.Errorf("encoding update to height %d: %w", target, err)
	}
	return update, nil
}

// checkUpdateTime checks whether a target is usable at the host time.
func checkUpdateTime(
	target *besu.Header,
	state besumsgs.IBesuLightClientMsgsClientState,
	hostSeconds uint64,
) error {
	if trustErr := checkTrustingPeriod(state.TrustingPeriod, target.Timestamp, hostSeconds); trustErr != nil {
		return fmt.Errorf("proof height %d: %w", target.Height, trustErr)
	}
	if exceedsClockDrift(target.Timestamp, hostSeconds, state.MaxClockDrift) {
		return fmt.Errorf("target height %d exceeds host clock drift", target.Height)
	}

	return nil
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
		proof, err := besu.EncodeMembershipProof(besumsgs.IBesuLightClientMsgsMembershipProof{
			ConsensusStatePreimage: snap.consensus,
			AccountProofNodes:      accountProof,
			ProofNodes:             storage.Proof,
		})
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
func (g *Generator) preimage(ctx context.Context, height uint64) (besumsgs.IBesuLightClientMsgsConsensusState, error) {
	header, err := g.header(ctx, height)
	if err != nil {
		return besumsgs.IBesuLightClientMsgsConsensusState{}, fmt.Errorf(
			"rebuilding trusted consensus state at height %d: %w",
			height,
			err,
		)
	}

	state := consensusOf(header)
	stored, err := g.host.ConsensusStateHash(ctx, g.clientID, height)
	if err != nil {
		return besumsgs.IBesuLightClientMsgsConsensusState{}, err
	}

	hash, err := besu.HashConsensusState(state)
	if err != nil {
		return besumsgs.IBesuLightClientMsgsConsensusState{}, err
	}

	if hash != common.Hash(stored) {
		return besumsgs.IBesuLightClientMsgsConsensusState{}, fmt.Errorf(
			"consensus state rebuilt for height %d hashes to %s but the light client stores %s",
			height, hash, common.Hash(stored),
		)
	}

	return state, nil
}
