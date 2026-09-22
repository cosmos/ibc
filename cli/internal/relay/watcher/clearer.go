// SPDX-License-Identifier: Apache-2.0

package watcher

import (
	"context"
	"log/slog"
	"slices"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/cli/internal/config"
	"github.com/cosmos/ibc/cli/internal/store"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

const chunkSizeSendEvents = 200

// ClearStore the persistence a clearing pass reads and writes.
type ClearStore interface {
	PacketStore

	ListPacketSequencesFrom(ctx context.Context, chainID string, clientID string, fromSequence uint64) ([]uint64, error)
	GetClearingState(ctx context.Context, chainID string, clientID string) (store.ClearingState, error)

	// Transact carries the pass's rows and its watermark, so a watermark is never
	// persisted on the strength of rows that did not land.
	Transact(ctx context.Context, call func(repo store.Repository) error) error
}

// Clearer recovers packets the subscription never saw. It asks the chain which
// packet commitments are still live and writes a row for each one we hold no
// record of, then records how far it probed so the next pass covers the new
// sequences rather than the client's whole history.
type Clearer struct {
	chainID string
	routes  map[string]config.ClientEnd
	chain   Chain
	storage ClearStore
	abandon bool
	logger  *slog.Logger
}

// Result the counts one clearing pass produced.
type Result struct {
	Probed      int
	Outstanding int
	AlreadyHeld int
	Recovered   int
	// Unresolved sequences the next pass will probe again.
	Unresolved int
	// Abandoned sequences held on record that no pass will probe again.
	Abandoned int
}

// client-specific clearer with fixed rpc latest block height
type clientClearer struct {
	*Clearer

	clientID          string
	latestBlockHeight uint64
}

func NewClearer(
	chainID string,
	connections []config.ConnectionConfig,
	chain Chain,
	storage ClearStore,
	cfg Config,
	logger *slog.Logger,
) *Clearer {
	return &Clearer{
		chainID: chainID,
		routes:  routesOf(chainID, connections),
		chain:   chain,
		storage: storage,
		abandon: cfg.AbandonUnrecoverablePackets,
		logger:  logger.With("module", "clearer", "chainID", chainID),
	}
}

// Clear clears packets for a client. Flow:
// 1. Take clientID and latest block height
// 2. Get existing rows from DB + the latest sequence from the chain
// 3. Based on sequence ranges and diffs, try to query commitments and rpc events
// 4. for outstanding (live onchain) sequences, store them in DB + update clearer state
//
// Note that websocket subscription runs concurrent to Clear() and pinning to a fixed RPC height is OK
// as packets/heights that are created in-flight, will be picked up by ws subscription.
func (c *Clearer) Clear(ctx context.Context, clientID string) (Result, error) {
	// pin RPC queries to the same block height
	latestBlockHeight, err := c.getLatestBlockHeight(ctx)
	if err != nil {
		return Result{}, errors.Wrapf(err, "reading the latest block height")
	}

	// clientID-specific clearer
	clientClearer := &clientClearer{
		Clearer:           c,
		clientID:          clientID,
		latestBlockHeight: latestBlockHeight,
	}

	r, err := clientClearer.clear(ctx)
	if err != nil {
		return Result{}, errors.Wrapf(err, "clientID %s", clientID)
	}

	return r, nil
}

func (c *Clearer) getLatestBlockHeight(ctx context.Context) (uint64, error) {
	head, err := c.chain.GetBlockHeader(ctx, v2.LatestBlock)
	if err != nil {
		return 0, errors.Wrapf(err, "reading the latest header")
	}

	return head.Height, nil
}

func (c *clientClearer) clear(ctx context.Context) (Result, error) {
	state, err := c.storage.GetClearingState(ctx, c.chainID, c.clientID)
	if err != nil {
		return Result{}, errors.Wrapf(err, "reading the clearing state")
	}

	// we start clearing from this sequence
	seqFrom := state.LastProbed + 1

	// latest sequence on the chain
	seqLatest, err := c.chain.LatestPacketSequence(ctx, c.clientID, c.latestBlockHeight)
	if err != nil {
		return Result{}, errors.Wrapf(err, "reading the latest sequence")
	}

	// query unresolved packages one more time. This might succeed ONLY
	// if an operator changes RPC to an archival one
	reprobe := state.Unresolved
	if c.abandon {
		reprobe = nil
	}

	seqsOutstanding, seqsQueried, err := c.queryLiveSequences(ctx, seqFrom, seqLatest, reprobe)
	if err != nil {
		return Result{}, err
	}

	unrecorded, err := c.buildUnrecorded(ctx, seqFrom, seqsOutstanding)
	if err != nil {
		return Result{}, err
	}

	toBeInserted, unresolved, err := c.querySendEventsAndBuildRows(ctx, unrecorded)
	if err != nil {
		return Result{}, err
	}

	if len(unresolved) > 0 {
		c.warnUnservable(unresolved)
	}

	// measured against what the pass actually probed: a sequence it carried
	// rather than probed had nothing learned about it, so it stays untouched
	delta := unresolvedDelta(reprobe, unresolved, c.latestBlockHeight)

	result := Result{
		Probed:      seqsQueried,
		Outstanding: len(seqsOutstanding),
		AlreadyHeld: len(seqsOutstanding) - len(unrecorded),
		Recovered:   len(toBeInserted),
		Unresolved:  0,
		Abandoned:   0,
	}

	if c.abandon {
		result.Abandoned = len(state.Unresolved) + len(delta.Add)
	} else {
		result.Unresolved = len(unresolved)
	}

	if err := c.persist(ctx, toBeInserted, seqLatest, delta); err != nil {
		return Result{}, err
	}

	return result, nil
}

func (c *clientClearer) warnUnservable(sequences []uint64) {
	if c.abandon {
		c.logger.Warn(
			"Abandoning packets whose send log no endpoint would serve: their escrow stays locked "+
				"and no pass will look at them again until abandonUnrecoverablePackets is turned off",
			"clientID", c.clientID,
			"sequences", sequences,
		)

		return
	}

	// nothing drains this set on an endpoint that has permanently pruned the
	// logs, so these are re-probed forever. Point the relayer at an archive
	// endpoint, or abandon them.
	c.logger.Warn(
		"Outstanding packets whose send log no endpoint would serve: their escrow stays locked "+
			"and every pass will retry them",
		"clientID", c.clientID,
		"sequences", sequences,
	)
}

// queryLiveSequences convert from+to into a range, and query their commitments on-chain.
// optionally query unresolved sequences -- maybe they'll be found live?
func (c *clientClearer) queryLiveSequences(
	ctx context.Context,
	seqFrom, seqTo uint64,
	unresolvedSequences []uint64,
) (liveSequences []uint64, queried int, err error) {
	queryCommitments := func(sequences []uint64) error {
		if len(sequences) == 0 {
			return nil
		}

		// len(found) <= len(chunk) because some seqs might be missing
		// note: evm client chunks queries automatically
		found, err := c.chain.PacketCommitments(ctx, c.clientID, sequences, c.latestBlockHeight)
		if err != nil {
			return errors.Wrapf(err, "probing packet commitments")
		}

		queried += len(sequences)
		liveSequences = append(liveSequences, found...)

		return nil
	}

	if err := queryCommitments(sequenceRange(seqFrom, seqTo)); err != nil {
		return nil, 0, err
	}

	if len(unresolvedSequences) > 0 {
		c.logger.Info(
			"Querying unresolved sequences",
			"clientID", c.clientID,
			"unresolved.length", len(unresolvedSequences),
			"unresolved.min", unresolvedSequences[0],
			"unresolved.max", unresolvedSequences[len(unresolvedSequences)-1],
		)

		if err := queryCommitments(unresolvedSequences); err != nil {
			return nil, 0, err
		}
	}

	return liveSequences, queried, nil
}

// buildUnrecorded constructs unrecorded seqs: outstanding (live onchain) minus already recorded in DB
func (c *clientClearer) buildUnrecorded(ctx context.Context, seqFrom uint64, outstanding []uint64) ([]uint64, error) {
	if len(outstanding) == 0 {
		return nil, nil
	}

	// todo pagination?
	recorded, err := c.storage.ListPacketSequencesFrom(ctx, c.chainID, c.clientID, seqFrom)
	if err != nil {
		return nil, errors.Wrapf(err, "listing recorded sequences")
	}

	set := make(map[uint64]struct{}, len(recorded))
	for _, sequence := range recorded {
		set[sequence] = struct{}{}
	}

	unrecorded := make([]uint64, 0, len(outstanding))

	for _, sequence := range outstanding {
		if _, ok := set[sequence]; !ok {
			unrecorded = append(unrecorded, sequence)
		}
	}

	return unrecorded, nil
}

// takes list of seqs, resolves onchain packet events and constructs rows to write.
// return rows and list of unresolved seqs (i.e. a seq has NO rpc event to resolve it)
func (c *clientClearer) querySendEventsAndBuildRows(ctx context.Context, sequences []uint64) (
	rows []store.UpsertPacket,
	unresolved []uint64,
	err error,
) {
	for chunk := range slices.Chunk(sequences, chunkSizeSendEvents) {
		events, err := c.chain.FindSendPackets(ctx, c.clientID, chunk)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "finding sends")
		}

		found := make(map[uint64]struct{}, len(events))

		for _, event := range events {
			// the endpoint served this send, so it is resolved whatever we
			// decide to do with it: the unresolved set is about logs no
			// endpoint would serve, not about packets we declined to record
			found[event.Packet.Sequence] = struct{}{}

			// a send naming a counterparty we do not relay to is not ours to
			// record, the same check the subscription path makes
			route, configured := c.routes[event.Packet.SourceClient]
			if !configured || event.Packet.DestinationClient != route.ClientID {
				c.logger.Warn(
					"Skipping outstanding packet with unconfigured destination client",
					"clientID", event.Packet.SourceClient,
					"destinationClientID", event.Packet.DestinationClient,
					"sequence", event.Packet.Sequence,
				)

				continue
			}

			rows = append(rows, packetRow(c.chainID, route.ChainID, event))
		}

		// a send whose log the endpoint no longer serves costs one packet;
		// failing the pass costs every packet behind it
		for _, sequence := range chunk {
			if _, ok := found[sequence]; !ok {
				unresolved = append(unresolved, sequence)
			}
		}
	}

	return rows, unresolved, nil
}

// update DB (clearer state + packet rows)
func (c *clientClearer) persist(
	ctx context.Context,
	rows []store.UpsertPacket,
	seqLastProbed uint64,
	delta store.UnresolvedDelta,
) error {
	return c.storage.Transact(ctx, func(repo store.Repository) error {
		for _, row := range rows {
			if err := repo.UpsertPacket(ctx, row); err != nil {
				return errors.Wrapf(err, "creating packet %d", row.PacketSequenceNumber)
			}
		}

		if err := repo.SetClearingState(ctx, c.chainID, c.clientID, seqLastProbed, delta); err != nil {
			return errors.Wrapf(err, "recording the clearing state")
		}
		return nil
	})
}

// unresolvedDelta is the change a pass makes to the unresolved set: what it
// found outstanding with no send log, and what it probed and no longer needs to
// remember. Sequences absent from both are left alone.
func unresolvedDelta(previousUnresolved, currentUnresolved []uint64, height uint64) store.UnresolvedDelta {
	var (
		delta       = store.UnresolvedDelta{Height: height}
		previousSet = make(map[uint64]struct{}, len(previousUnresolved))
		currentSet  = make(map[uint64]struct{}, len(currentUnresolved))
	)

	for _, sequence := range previousUnresolved {
		previousSet[sequence] = struct{}{}
	}

	for _, sequence := range currentUnresolved {
		currentSet[sequence] = struct{}{}

		// net-new sequence is considered "unresolved"
		if _, ok := previousSet[sequence]; !ok {
			delta.Add = append(delta.Add, sequence)
		}
	}

	for _, sequence := range previousUnresolved {
		// sequence is no longer "unresolved"
		if _, ok := currentSet[sequence]; !ok {
			delta.Resolve = append(delta.Resolve, sequence)
		}
	}

	return delta
}

func sequenceRange(from, to uint64) []uint64 {
	if from > to {
		return []uint64{}
	}

	sequences := make([]uint64, 0, to-from+1)
	for sequence := from; sequence <= to; sequence++ {
		sequences = append(sequences, sequence)
	}

	return sequences
}
