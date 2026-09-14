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
	LatestPacketSequence(ctx context.Context, sourceClientID string, height uint64) (uint64, error)
	PacketCommitments(ctx context.Context, sourceClientID string, sequences []uint64, height uint64) ([]uint64, error)
	FindSendPackets(ctx context.Context, sourceClientID string, sequences []uint64) ([]v2.PacketEvent, error)
}

// ClearStore the persistence a clearing pass reads and writes.
type ClearStore interface {
	PacketStore

	MaxPacketSequence(ctx context.Context, chainID string, clientID string) (uint64, error)
	ListPacketSequencesFrom(ctx context.Context, chainID string, clientID string, fromSequence uint64) ([]uint64, error)
}

// Result the counts one clearing pass produced.
type Result struct {
	Probed      int
	Outstanding int
	AlreadyHeld int
	Recovered   int
	// Unresolved sequences the next pass will probe again.
	Unresolved int
}

// Clearer recovers packets the subscription never saw. It asks the chain which
// packet commitments are still live and writes a row for each one we hold no
// record of.
type Clearer struct {
	chainID string
	routes  map[string]config.ClientEnd
	chain   OutstandingQuerier
	storage ClearStore
	logger  *slog.Logger
}

func NewClearer(
	chainID string,
	connections []config.ConnectionConfig,
	chain OutstandingQuerier,
	storage ClearStore,
	logger *slog.Logger,
) *Clearer {
	return &Clearer{
		chainID: chainID,
		routes:  routesOf(chainID, connections),
		chain:   chain,
		storage: storage,
		logger:  logger.With("module", "clearer", "chainID", chainID),
	}
}

// Clear writes a packet row for every outstanding commitment on clientID we
// hold no row for. Sequences we already know about are skipped whatever state
// they are in, terminal and deselected ones included: clearing discovers
// packets, it does not reopen decisions the pipeline or an operator has
// already made.
func (c *Clearer) Clear(ctx context.Context, clientID string) (Result, error) {
	var result Result

	latest, err := c.chain.LatestPacketSequence(ctx, clientID, v2.LatestBlock)
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
	// either way: a sequence this low probes a shorter range, it writes nothing off
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

	outstanding, probed, err := c.outstanding(ctx, clientID, latest)
	if err != nil {
		return result, err
	}

	result.Probed, result.Outstanding = probed, len(outstanding)

	unrecorded, err := c.unrecorded(ctx, clientID, outstanding)
	if err != nil {
		return result, err
	}

	result.AlreadyHeld = len(outstanding) - len(unrecorded)

	rows, unresolved, err := c.sends(ctx, clientID, unrecorded)
	if err != nil {
		return result, err
	}

	result.Recovered, result.Unresolved = len(rows), len(unresolved)

	if len(unresolved) > 0 {
		// nothing drains this set on an endpoint that has permanently pruned the
		// logs, so these are re-probed forever. Point the relayer at an archive
		// endpoint.
		c.logger.Warn(
			"Outstanding packets whose send log no endpoint would serve: their escrow stays locked "+
				"and every pass will retry them",
			"clientID", clientID,
			"sequences", unresolved,
		)
	}

	return result, c.persist(ctx, clientID, rows)
}

// outstanding probes the client's sequence space and returns those whose
// commitment is still live, with the number probed. The reads stay at the head:
// a commitment written above the finalized head reads absent there, and absent
// means settled.
func (c *Clearer) outstanding(ctx context.Context, clientID string, latest uint64) ([]uint64, int, error) {
	var (
		live   []uint64
		probed int
	)

	for lo := uint64(1); lo <= latest; lo += probeChunk {
		chunk := sequenceRange(lo, min(lo+probeChunk-1, latest))

		found, err := c.chain.PacketCommitments(ctx, clientID, chunk, v2.LatestBlock)
		if err != nil {
			return nil, 0, errors.Wrapf(err, "probing packet commitments for client %s", clientID)
		}

		probed += len(chunk)
		live = append(live, found...)
	}

	return live, probed, nil
}

// unrecorded drops the sequences we already hold a row for.
func (c *Clearer) unrecorded(ctx context.Context, clientID string, outstanding []uint64) ([]uint64, error) {
	if len(outstanding) == 0 {
		return nil, nil
	}

	recorded, err := c.storage.ListPacketSequencesFrom(ctx, c.chainID, clientID, 1)
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
// along with the sequences whose send log the endpoint would not serve.
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

// persist writes the pass's rows. A pass that dies halfway through leaves the
// rows it did write, and the next one covers the rest: nothing here depends on
// the whole set landing at once.
func (c *Clearer) persist(ctx context.Context, clientID string, rows []store.UpsertPacket) error {
	for _, row := range rows {
		if err := c.storage.UpsertPacket(ctx, row); err != nil {
			return errors.Wrapf(
				err,
				"creating packet %d for client %s",
				row.PacketSequenceNumber, clientID,
			)
		}
	}

	return nil
}

func sequenceRange(from, to uint64) []uint64 {
	sequences := make([]uint64, 0, to-from+1)
	for sequence := from; sequence <= to; sequence++ {
		sequences = append(sequences, sequence)
	}

	return sequences
}
