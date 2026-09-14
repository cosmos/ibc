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

const (
	// probeChunk bounds the range this pass materializes at once, so a client
	// with a long history costs one chunk rather than its whole sequence space.
	// How many probes fit in one call is the chain client's business.
	probeChunk = 1000

	// sendLogChunk bounds the sequences in one FindSendPackets query, since
	// providers differ on how long a topic OR-list they accept.
	sendLogChunk = 200
)

// OutstandingQuerier the chain reads a clearing pass makes. It excludes
// SubscribeSendPackets deliberately: the backstop exists to cover the gaps the
// tip leaves, so it must compile, and keep working, without one.
type OutstandingQuerier interface {
	GetBlockHeader(ctx context.Context, height uint64) (v2.BlockHeader, error)
	LatestPacketSequence(ctx context.Context, sourceClientID string, height uint64) (uint64, error)
	PacketCommitments(ctx context.Context, sourceClientID string, sequences []uint64, height uint64) ([]uint64, error)
	FindSendPackets(ctx context.Context, sourceClientID string, sequences []uint64) ([]v2.PacketEvent, error)
}

// ClearStore the persistence a clearing pass reads and writes.
type ClearStore interface {
	PacketStore

	MaxPacketSequence(ctx context.Context, chainID string, clientID string) (uint64, error)
	ListPacketSequencesFrom(ctx context.Context, chainID string, clientID string, fromSequence uint64) ([]uint64, error)
	GetClearingState(ctx context.Context, chainID string, clientID string) (store.ClearingState, error)

	// Transact carries the pass's rows and its watermark, so a watermark is never
	// persisted on the strength of rows that did not land.
	Transact(ctx context.Context, call func(repo store.Repository) error) error
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

// Clearer recovers packets the subscription never saw. It asks the chain which
// packet commitments are still live and writes a row for each one we hold no
// record of, then records how far it probed so the next pass covers the new
// sequences rather than the client's whole history.
type Clearer struct {
	chainID string
	routes  map[string]config.ClientEnd
	chain   OutstandingQuerier
	storage ClearStore
	abandon bool
	logger  *slog.Logger
}

func NewClearer(
	chainID string,
	connections []config.ConnectionConfig,
	chain OutstandingQuerier,
	storage ClearStore,
	clearing ClearConfig,
	logger *slog.Logger,
) *Clearer {
	return &Clearer{
		chainID: chainID,
		routes:  routesOf(chainID, connections),
		chain:   chain,
		storage: storage,
		abandon: clearing.AbandonUnrecoverablePackets,
		logger:  logger.With("module", "clearer", "chainID", chainID),
	}
}

// Clear writes a packet row for every outstanding commitment on clientID we
// hold no row for, probing the sequences above the watermark plus whatever the
// last pass could not resolve. Sequences we already know about are skipped
// whatever state they are in, terminal and deselected ones included: clearing
// discovers packets, it does not reopen decisions the pipeline or an operator
// has already made.
func (c *Clearer) Clear(ctx context.Context, clientID string) (Result, error) {
	var result Result

	// every read this pass makes names a block number rather than the latest tag:
	// behind a load-balanced endpoint the tag resolves on whichever node serves
	// the call, and a node that has not caught up reads a live commitment as
	// absent, which the pass would take for settled
	head, err := c.chain.GetBlockHeader(ctx, v2.LatestBlock)
	if err != nil {
		return result, errors.Wrapf(err, "reading the latest header for client %s", clientID)
	}

	latest, err := c.chain.LatestPacketSequence(ctx, clientID, head.Height)
	if err != nil {
		return result, errors.Wrapf(err, "reading the latest sequence for client %s", clientID)
	}

	recorded, err := c.storage.MaxPacketSequence(ctx, c.chainID, clientID)
	if err != nil {
		return result, errors.Wrapf(err, "reading the highest recorded sequence for client %s", clientID)
	}

	// the chain cannot have sent fewer packets than we hold rows for, but the
	// subscription reads a different endpoint than this pass does, so an rpc node
	// a block behind the websocket one produces this too. The pass carries on
	// either way: a sequence this low probes no new range, and the watermark
	// upsert refuses to move backwards
	if latest < recorded {
		c.logger.Warn(
			"The chain reports fewer sends than we hold packets for. Either the rpc endpoint is "+
				"behind the one the subscription reads, the client was reset, or the IBCStore "+
				"storage layout has moved",
			"clientID", clientID,
			"chainSequence", latest,
			"recordedSequence", recorded,
		)
	}

	state, err := c.storage.GetClearingState(ctx, c.chainID, clientID)
	if err != nil {
		return result, errors.Wrapf(err, "reading the clearing state for client %s", clientID)
	}

	from := state.LastProbed + 1

	// abandoned sequences are carried rather than probed: dropping them from the
	// probe is what bounds the pass, and keeping them on record is what lets an
	// operator who later has an archive endpoint recover them by turning the
	// setting back off.
	// ponytail: the abandoned set is read every pass and thrown away; make the
	// query aware of the setting if anyone parks enough of them to notice.
	reprobe := state.Unresolved
	if c.abandon {
		reprobe = nil
	}

	outstanding, probed, probeHeight, err := c.outstanding(ctx, clientID, reprobe, from, latest, head.Height)
	if err != nil {
		return result, err
	}

	result.Probed, result.Outstanding = probed, len(outstanding)

	unrecorded, err := c.unrecorded(ctx, clientID, from, outstanding)
	if err != nil {
		return result, err
	}

	result.AlreadyHeld = len(outstanding) - len(unrecorded)

	rows, unresolved, err := c.sends(ctx, clientID, unrecorded)
	if err != nil {
		return result, err
	}

	result.Recovered = len(rows)

	if len(unresolved) > 0 {
		c.warnUnservable(clientID, unresolved)
	}

	// measured against what the pass actually probed: a sequence it carried
	// rather than probed had nothing learned about it, so it stays untouched
	delta := unresolvedDelta(reprobe, unresolved, probeHeight)

	if c.abandon {
		result.Abandoned = len(state.Unresolved) + len(delta.Add)
	} else {
		result.Unresolved = len(unresolved)
	}

	return result, c.persist(ctx, clientID, rows, latest, delta)
}

// unresolvedDelta is the change a pass makes to the unresolved set: what it
// found outstanding with no send log, and what it probed and no longer needs to
// remember. Sequences absent from both are left alone.
func unresolvedDelta(probed, unresolved []uint64, height uint64) store.UnresolvedDelta {
	delta := store.UnresolvedDelta{Height: height}

	held := make(map[uint64]struct{}, len(probed))
	for _, sequence := range probed {
		held[sequence] = struct{}{}
	}

	stuck := make(map[uint64]struct{}, len(unresolved))

	for _, sequence := range unresolved {
		stuck[sequence] = struct{}{}

		if _, ok := held[sequence]; !ok {
			delta.Add = append(delta.Add, sequence)
		}
	}

	for _, sequence := range probed {
		if _, ok := stuck[sequence]; !ok {
			delta.Resolve = append(delta.Resolve, sequence)
		}
	}

	return delta
}

func (c *Clearer) warnUnservable(clientID string, sequences []uint64) {
	if c.abandon {
		c.logger.Warn(
			"Abandoning packets whose send log no endpoint would serve: their escrow stays locked "+
				"and no pass will look at them again until abandonUnrecoverablePackets is turned off",
			"clientID", clientID,
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
		"clientID", clientID,
		"sequences", sequences,
	)
}

// outstanding probes the sequences the watermark has not settled and returns
// those whose commitment is still live, with the number probed and the height
// the pass ended at. The reads stay at the head: a commitment written above the
// finalized head reads absent there, and absent means settled.
func (c *Clearer) outstanding(
	ctx context.Context,
	clientID string,
	unresolved []uint64,
	from, latest, height uint64,
) ([]uint64, int, uint64, error) {
	var (
		live   []uint64
		probed int
	)

	probe := func(chunk []uint64) error {
		// a cold pass runs longer than the ~128 blocks of state a non-archive
		// node keeps, so the height moves up with the chain rather than aging
		// out from under the later chunks. max, because a head read served by a
		// lagging node must not drag it back below where the sequence was read
		head, err := c.chain.GetBlockHeader(ctx, v2.LatestBlock)
		if err != nil {
			return errors.Wrapf(err, "refreshing the probe height for client %s", clientID)
		}

		height = max(height, head.Height)

		found, err := c.chain.PacketCommitments(ctx, clientID, chunk, height)
		if err != nil {
			return errors.Wrapf(err, "probing packet commitments for client %s", clientID)
		}

		probed += len(chunk)
		live = append(live, found...)

		return nil
	}

	if len(unresolved) > 0 {
		if err := probe(unresolved); err != nil {
			return nil, 0, 0, err
		}
	}

	for lo := from; lo <= latest; lo += probeChunk {
		if err := probe(sequenceRange(lo, min(lo+probeChunk-1, latest))); err != nil {
			return nil, 0, 0, err
		}
	}

	return live, probed, height, nil
}

// unrecorded drops the sequences we already hold a row for. The query is bounded
// by the probe's own floor, so an unresolved sequence below it goes unchecked: it
// normally has no row, and where another instance recorded one first UpsertPacket
// leaves that row standing rather than duplicating it.
func (c *Clearer) unrecorded(
	ctx context.Context,
	clientID string,
	from uint64,
	outstanding []uint64,
) ([]uint64, error) {
	if len(outstanding) == 0 {
		return nil, nil
	}

	recorded, err := c.storage.ListPacketSequencesFrom(ctx, c.chainID, clientID, from)
	if err != nil {
		return nil, errors.Wrapf(err, "listing recorded sequences for client %s", clientID)
	}

	known := make(map[uint64]struct{}, len(recorded))
	for _, sequence := range recorded {
		known[sequence] = struct{}{}
	}

	unrecorded := make([]uint64, 0, len(outstanding))

	for _, sequence := range outstanding {
		if _, ok := known[sequence]; !ok {
			unrecorded = append(unrecorded, sequence)
		}
	}

	return unrecorded, nil
}

// sends looks up the send of every sequence and returns the rows to write,
// along with the sequences whose send log the endpoint would not serve. Those
// carry no row, so the watermark cannot be allowed to swallow them.
func (c *Clearer) sends(
	ctx context.Context,
	clientID string,
	sequences []uint64,
) (rows []store.UpsertPacket, unresolved []uint64, err error) {
	for chunk := range slices.Chunk(sequences, sendLogChunk) {
		events, errFind := c.chain.FindSendPackets(ctx, clientID, chunk)
		if errFind != nil {
			return nil, nil, errors.Wrapf(errFind, "finding sends for client %s", clientID)
		}

		found := make(map[uint64]struct{}, len(events))

		for _, event := range events {
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
			found[event.Packet.Sequence] = struct{}{}
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

// persist writes the pass's rows and its watermark together, so a watermark
// never outlives the rows it was earned by.
func (c *Clearer) persist(
	ctx context.Context,
	clientID string,
	rows []store.UpsertPacket,
	watermark uint64,
	delta store.UnresolvedDelta,
) error {
	return c.storage.Transact(ctx, func(repo store.Repository) error {
		for _, row := range rows {
			if err := repo.UpsertPacket(ctx, row); err != nil {
				return errors.Wrapf(
					err,
					"creating packet %d for client %s",
					row.PacketSequenceNumber, clientID,
				)
			}
		}

		if err := repo.SetClearingState(ctx, c.chainID, clientID, watermark, delta); err != nil {
			return errors.Wrapf(
				err,
				"recording the clearing state for client %s",
				clientID,
			)
		}
		return nil
	})
}

func sequenceRange(from, to uint64) []uint64 {
	sequences := make([]uint64, 0, to-from+1)
	for sequence := from; sequence <= to; sequence++ {
		sequences = append(sequences, sequence)
	}

	return sequences
}
