// SPDX-License-Identifier: Apache-2.0

package watcher

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/internal/store"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

// waitFor bounds how long a test blocks on a clearing pass's own goroutine.
const waitFor = 5 * time.Second

func newTestClearer(chain OutstandingQuerier, storage ClearStore) *Clearer {
	return newClearer(chain, storage, ClearConfig{})
}

func newClearer(chain OutstandingQuerier, storage ClearStore, clearing ClearConfig) *Clearer {
	return NewClearer(sourceChainID, testConnections(), chain, storage, clearing, slog.Default())
}

// fakeChain models what one chain has sent and what is still committed at a
// given height. It is written out rather than generated because a pass reads it
// several times and the answers have to stay consistent with each other: a
// settled sequence has to read as settled from every query the pass makes.
type fakeChain struct {
	mu     sync.Mutex
	head   uint64
	sentAt map[uint64]uint64
	// headReads are served to successive latest-header reads before head is,
	// so a test can hand the pass a head that moves backwards
	headReads []uint64

	settled   map[uint64]struct{}
	pruned    map[uint64]struct{}
	latestErr error
	headErr   error
	probeErr  error
	passes    int
	probes    [][]uint64
	heights   []uint64
	finds     [][]uint64
	gate      chan struct{}
}

func newFakeChain() *fakeChain {
	return &fakeChain{
		sentAt:  make(map[uint64]uint64),
		settled: make(map[uint64]struct{}),
		pruned:  make(map[uint64]struct{}),
	}
}

// send records sequences as sent at the current head and still outstanding.
func (c *fakeChain) send(sequences ...uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, sequence := range sequences {
		c.sentAt[sequence] = c.head
	}
}

// mine advances the head, so sends after it read as assigned above an earlier height.
func (c *fakeChain) mine(blocks uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.head += blocks
}

// sequenceAt is the highest sequence assigned at or below height.
func (c *fakeChain) sequenceAt(height uint64) uint64 {
	var latest uint64

	for sequence, at := range c.sentAt {
		if at <= height {
			latest = max(latest, sequence)
		}
	}

	return latest
}

func (c *fakeChain) sentBy(sequence, height uint64) bool {
	at, ok := c.sentAt[sequence]

	return ok && at <= height
}

func (c *fakeChain) sent(sequence uint64) bool {
	_, ok := c.sentAt[sequence]

	return ok
}

// settle deletes the packet commitments, as an ack or a timeout does.
func (c *fakeChain) settle(sequences ...uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, sequence := range sequences {
		c.settled[sequence] = struct{}{}
	}
}

// prune drops sends from the log index while leaving their commitments live.
func (c *fakeChain) prune(sequences ...uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, sequence := range sequences {
		c.pruned[sequence] = struct{}{}
	}
}

// unprune serves the sends again, as pointing the relayer at an archive endpoint does.
func (c *fakeChain) unprune(sequences ...uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, sequence := range sequences {
		delete(c.pruned, sequence)
	}
}

// hold blocks every clearing pass until the returned func releases them. It
// ignores the context on purpose, so a test can assert Stop waits for a pass
// that has not noticed the cancellation yet.
func (c *fakeChain) hold() func() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.gate = make(chan struct{})
	gate := c.gate

	return func() { close(gate) }
}

func (c *fakeChain) failLatest(err error)   { c.mu.Lock(); c.latestErr = err; c.mu.Unlock() }
func (c *fakeChain) failHead(err error)     { c.mu.Lock(); c.headErr = err; c.mu.Unlock() }
func (c *fakeChain) failProbe(err error)    { c.mu.Lock(); c.probeErr = err; c.mu.Unlock() }
func (c *fakeChain) passCount() int         { c.mu.Lock(); defer c.mu.Unlock(); return c.passes }
func (c *fakeChain) probeCalls() [][]uint64 { c.mu.Lock(); defer c.mu.Unlock(); return c.probes }
func (c *fakeChain) findCalls() [][]uint64  { c.mu.Lock(); defer c.mu.Unlock(); return c.finds }
func (c *fakeChain) probeHeights() []uint64 { c.mu.Lock(); defer c.mu.Unlock(); return c.heights }

// serveHeads queues the heights successive latest-header reads report.
func (c *fakeChain) serveHeads(heights ...uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.headReads = heights
}

func (c *fakeChain) GetBlockHeader(_ context.Context, height uint64) (v2.BlockHeader, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.headErr != nil {
		return v2.BlockHeader{}, c.headErr
	}

	if height == v2.LatestBlock {
		height = c.head

		if len(c.headReads) > 0 {
			height, c.headReads = c.headReads[0], c.headReads[1:]
		}
	}

	return v2.BlockHeader{Height: height, Timestamp: blockTime}, nil
}

// LatestPacketSequence answers for the height it is asked about, which is what
// makes a pinned height mean anything. Every pass reads it exactly once.
func (c *fakeChain) LatestPacketSequence(_ context.Context, _ string, height uint64) (uint64, error) {
	c.mu.Lock()

	c.passes++
	latest, err, gate := c.sequenceAt(height), c.latestErr, c.gate
	c.mu.Unlock()

	if gate != nil {
		<-gate
	}

	return latest, err
}

// PacketCommitments answers as a node at height would: a send assigned above it
// has no commitment there yet.
func (c *fakeChain) PacketCommitments(
	_ context.Context,
	_ string,
	sequences []uint64,
	height uint64,
) ([]uint64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.probes = append(c.probes, slices.Clone(sequences))
	c.heights = append(c.heights, height)

	if c.probeErr != nil {
		return nil, c.probeErr
	}

	var live []uint64

	for _, sequence := range sequences {
		if _, gone := c.settled[sequence]; c.sentBy(sequence, height) && !gone {
			live = append(live, sequence)
		}
	}

	return live, nil
}

func (c *fakeChain) FindSendPackets(_ context.Context, _ string, sequences []uint64) ([]v2.PacketEvent, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.finds = append(c.finds, slices.Clone(sequences))

	var events []v2.PacketEvent

	for _, sequence := range sequences {
		if _, gone := c.pruned[sequence]; c.sent(sequence) && !gone {
			events = append(events, sendPacketEvent(sequence))
		}
	}

	return events, nil
}

// failingWrites fails every row write inside the pass's transaction, leaving
// the real store to roll back what the pass had already written.
type failingWrites struct {
	ClearStore

	err error
}

func (s failingWrites) Transact(ctx context.Context, call func(store.Repository) error) error {
	return s.ClearStore.Transact(ctx, func(repo store.Repository) error {
		return call(failingRepo{Repository: repo, err: s.err})
	})
}

type failingRepo struct {
	store.Repository

	err error
}

func (r failingRepo) UpsertPacket(context.Context, store.UpsertPacket) error { return r.err }

// boundedStore records the floor the clearing pass queries recorded sequences with.
type boundedStore struct {
	ClearStore

	from uint64
}

func (s *boundedStore) ListPacketSequencesFrom(
	ctx context.Context,
	chainID, clientID string,
	fromSequence uint64,
) ([]uint64, error) {
	s.from = fromSequence

	return s.ClearStore.ListPacketSequencesFrom(ctx, chainID, clientID, fromSequence)
}

// clearingState is how far the pass recorded probing, which is the bound the
// next one starts from.
func clearingState(t *testing.T, db *store.SqliteDB) store.ClearingState {
	t.Helper()

	state, err := db.GetClearingState(context.Background(), sourceChainID, sourceClientID)
	require.NoError(t, err)

	return state
}

// watcherStore is the real store a clearing pass reads and writes, which is
// most of what a pass does.
func watcherStore(t *testing.T) *store.SqliteDB {
	t.Helper()

	db, err := store.NewSqliteInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.MigrateUp()
	require.NoError(t, err)

	return db
}

// recorded is the sequences the store holds for the watched client.
func recorded(t *testing.T, db *store.SqliteDB) []uint64 {
	t.Helper()

	sequences, err := db.ListPacketSequencesFrom(context.Background(), sourceChainID, sourceClientID, 1)
	require.NoError(t, err)

	return sequences
}

// hold pre-seeds a row the clearing pass must treat as already known.
func hold(t *testing.T, db *store.SqliteDB, sequence uint64, status store.RelayStatus) {
	t.Helper()

	ctx := context.Background()
	row := packetRow(sourceChainID, destChainID, sendPacketEvent(sequence))
	require.NoError(t, db.UpsertPacket(ctx, row))

	if status != store.RelayStatusPending {
		key := store.PacketKey{
			SourceChainID:  sourceChainID,
			SourceClientID: sourceClientID,
			Sequence:       sequence,
		}
		require.NoError(t, db.UpdatePacketStatus(ctx, key, status))
	}
}

func TestClearerClear(t *testing.T) {
	ctx := context.Background()

	t.Run("outstandingCommitmentsGetRows", func(t *testing.T) {
		chain := newFakeChain()
		chain.send(1, 2, 3, 4, 5)
		chain.settle(1, 2, 4)

		db := watcherStore(t)

		result, err := newTestClearer(chain, db).Clear(ctx, sourceClientID)
		require.NoError(t, err)

		assert.Equal(t, Result{Probed: 5, Outstanding: 2, Recovered: 2}, result)
		assert.Equal(t, []uint64{3, 5}, recorded(t, db))
		assert.Equal(t, store.ClearingState{LastProbed: 5}, clearingState(t, db))

		rows, err := db.ListPacketsBySourceTx(ctx, sourceChainID, sendTxHash)
		require.NoError(t, err)
		require.NotEmpty(t, rows)

		row := rows[0]
		assert.Equal(t, store.RelayStatusPending, row.Status)
		assert.Equal(t, sourceChainID, row.SourceChainID)
		assert.Equal(t, destChainID, row.DestinationChainID)
		assert.Equal(t, sourceClientID, row.PacketSourceClientID)
		assert.Equal(t, destClientID, row.PacketDestinationClientID)
		assert.Equal(t, sendTxHash, row.SourceTxHash)
		assert.Equal(t, blockTime, row.SourceTxTime.UTC())
	})

	t.Run("settledCommitmentsWriteNothing", func(t *testing.T) {
		chain := newFakeChain()
		chain.send(1, 2, 3)
		chain.settle(1, 2, 3)

		db := watcherStore(t)

		result, err := newTestClearer(chain, db).Clear(ctx, sourceClientID)
		require.NoError(t, err)

		assert.Equal(t, Result{Probed: 3}, result)
		assert.Empty(t, recorded(t, db))
		assert.Empty(t, chain.findCalls())
	})

	t.Run("sequencesWeHoldAreSkippedInEveryState", func(t *testing.T) {
		for _, status := range []store.RelayStatus{
			store.RelayStatusPending,
			store.RelayStatusNotSelected,
			store.RelayStatusFailed,
			store.RelayStatusCompleteWithTimeout,
			store.RelayStatusCompleteWithAck,
		} {
			t.Run(string(status), func(t *testing.T) {
				chain := newFakeChain()
				chain.send(1, 2)

				db := watcherStore(t)
				hold(t, db, 1, status)

				result, err := newTestClearer(chain, db).Clear(ctx, sourceClientID)
				require.NoError(t, err)

				assert.Equal(t, Result{Probed: 2, Outstanding: 2, AlreadyHeld: 1, Recovered: 1}, result)
				assert.Equal(t, []uint64{1, 2}, recorded(t, db))
				assert.Equal(t, [][]uint64{{2}}, chain.findCalls())
			})
		}
	})

	t.Run("theSkipQueryIsBoundedByTheWatermark", func(t *testing.T) {
		chain := newFakeChain()
		chain.send(1, 2, 3, 4)

		db := watcherStore(t)
		require.NoError(t, db.SetClearingState(ctx, sourceChainID, sourceClientID, 2, store.UnresolvedDelta{}))

		storage := &boundedStore{ClearStore: db}

		_, err := newTestClearer(chain, storage).Clear(ctx, sourceClientID)
		require.NoError(t, err)

		assert.Equal(t, uint64(3), storage.from)
	})

	// the rpc endpoint this pass reads lags the websocket the subscription reads,
	// so the sequence counter trails rows the subscription already wrote. Nothing
	// is written off by carrying on: the range is bounded by what the chain reports
	t.Run("aChainBehindOurRowsKeepsClearing", func(t *testing.T) {
		chain := newFakeChain()
		chain.send(1, 2)

		db := watcherStore(t)
		hold(t, db, 7, store.RelayStatusPending)

		result, err := newTestClearer(chain, db).Clear(ctx, sourceClientID)
		require.NoError(t, err)

		assert.Equal(t, 2, result.Probed)
		assert.Equal(t, []uint64{1, 2}, chain.probeCalls()[0])
		assert.Equal(t, uint64(2), clearingState(t, db).LastProbed)
	})

	// a pass that read a stale watermark reports the bound it probed to, and the
	// store keeps the higher one another instance already earned
	t.Run("aWatermarkBehindTheStoredOneDoesNotMoveIt", func(t *testing.T) {
		chain := newFakeChain()
		chain.send(1, 2, 3)

		db := watcherStore(t)
		require.NoError(t, db.SetClearingState(ctx, sourceChainID, sourceClientID, 9, store.UnresolvedDelta{}))

		_, err := newTestClearer(chain, db).Clear(ctx, sourceClientID)
		require.NoError(t, err)

		assert.Equal(t, uint64(9), clearingState(t, db).LastProbed)
	})

	t.Run("aClientWithNothingSentProbesNothing", func(t *testing.T) {
		chain := newFakeChain()

		result, err := newTestClearer(chain, watcherStore(t)).Clear(ctx, sourceClientID)
		require.NoError(t, err)

		assert.Equal(t, Result{}, result)
		assert.Empty(t, chain.probeCalls())
	})

	t.Run("probeFailureWritesNothing", func(t *testing.T) {
		chain := newFakeChain()
		chain.send(1, 2)
		chain.failProbe(errors.New("rpc refused the batch"))

		db := watcherStore(t)

		_, err := newTestClearer(chain, db).Clear(ctx, sourceClientID)
		require.ErrorContains(t, err, "rpc refused the batch")
		assert.Empty(t, recorded(t, db))
		assert.Equal(t, store.ClearingState{}, clearingState(t, db))
	})

	t.Run("bothQueriesAreChunked", func(t *testing.T) {
		chain := newFakeChain()

		const sent = probeChunk + sendLogChunk

		for sequence := uint64(1); sequence <= sent; sequence++ {
			chain.send(sequence)
		}

		result, err := newTestClearer(chain, watcherStore(t)).Clear(ctx, sourceClientID)
		require.NoError(t, err)

		assert.Equal(t, sent, result.Probed)
		assert.Equal(t, sent, result.Recovered)

		assertCovers(t, chain.probeCalls(), probeChunk, sent)
		assertCovers(t, chain.findCalls(), sendLogChunk, sent)
	})

	t.Run("theSequenceIsReadBeforeTheProbe", func(t *testing.T) {
		chain := newFakeChain()
		chain.send(1, 2)

		release := chain.hold()
		done := make(chan Result, 1)

		db := watcherStore(t)

		go func() {
			result, err := newTestClearer(chain, db).Clear(ctx, sourceClientID)
			assert.NoError(t, err)
			done <- result
		}()

		require.Eventually(t, func() bool { return chain.passCount() == 1 }, waitFor, time.Millisecond)

		// probing above a sequence read after the probe would write off the
		// packets sent in between, so the read has to come first
		assert.Empty(t, chain.probeCalls())

		release()

		assert.Equal(t, 2, (<-done).Probed)
	})

	t.Run("aWarmPassProbesOnlyWhatTheWatermarkHasNotSettled", func(t *testing.T) {
		chain := newFakeChain()

		const sent = 2 * probeChunk

		for sequence := uint64(1); sequence <= sent; sequence++ {
			chain.send(sequence)
		}

		// everything is settled but one packet, which stays stuck in a terminal
		// state: a client's history must not pin the probe to it
		chain.settle(sequenceRange(2, sent)...)

		clearer := newTestClearer(chain, watcherStore(t))

		cold, err := clearer.Clear(ctx, sourceClientID)
		require.NoError(t, err)
		assert.Equal(t, sent, cold.Probed)

		coldCalls := len(chain.probeCalls())
		require.Equal(t, 2, coldCalls)

		chain.send(sent + 1)

		warm, err := clearer.Clear(ctx, sourceClientID)
		require.NoError(t, err)

		assert.Equal(t, 1, warm.Probed)
		assert.Equal(t, [][]uint64{{sent + 1}}, chain.probeCalls()[coldCalls:])
		assert.Less(t, len(chain.probeCalls())-coldCalls, coldCalls)
	})

	t.Run("anUnresolvedSendIsProbedUntilItSettles", func(t *testing.T) {
		chain := newFakeChain()
		chain.send(1, 2)
		chain.prune(1)

		db := watcherStore(t)
		clearer := newTestClearer(chain, db)

		result, err := clearer.Clear(ctx, sourceClientID)
		require.NoError(t, err)

		// the pruned send has no row and sits below the watermark, so only the
		// unresolved set keeps it in the probe
		assert.Equal(t, Result{Probed: 2, Outstanding: 2, Recovered: 1, Unresolved: 1}, result)
		assert.Equal(t, []uint64{2}, recorded(t, db))
		assert.Equal(t, store.ClearingState{LastProbed: 2, Unresolved: []uint64{1}}, clearingState(t, db))

		before := len(chain.probeCalls())

		result, err = clearer.Clear(ctx, sourceClientID)
		require.NoError(t, err)

		assert.Equal(t, [][]uint64{{1}}, chain.probeCalls()[before:])
		assert.Equal(t, 1, result.Unresolved)

		// an ack or a timeout deletes the commitment, which is the only thing
		// that drains the set
		chain.settle(1)

		result, err = clearer.Clear(ctx, sourceClientID)
		require.NoError(t, err)

		assert.Zero(t, result.Unresolved)
		assert.Equal(t, store.ClearingState{LastProbed: 2}, clearingState(t, db))
	})

	t.Run("aLaggingHeadCannotResolveAnUnresolvedSend", func(t *testing.T) {
		chain := newFakeChain()
		chain.mine(10)
		chain.send(1, 2)
		chain.prune(1)

		db := watcherStore(t)
		clearer := newTestClearer(chain, db)

		_, err := clearer.Clear(ctx, sourceClientID)
		require.NoError(t, err)

		assert.Equal(t, store.ClearingState{LastProbed: 2, Unresolved: []uint64{1}}, clearingState(t, db))

		// the next pass opens on a node four blocks behind the send, which reads
		// the live commitment as absent. Resolving on that would drop the only
		// record of a packet sitting below the watermark
		chain.serveHeads(6, 6)

		_, err = clearer.Clear(ctx, sourceClientID)
		require.NoError(t, err)

		assert.Equal(t, store.ClearingState{LastProbed: 2, Unresolved: []uint64{1}}, clearingState(t, db))

		// a pass that reads at or above the height the commitment was last seen
		// live at still resolves it
		chain.settle(1)

		_, err = clearer.Clear(ctx, sourceClientID)
		require.NoError(t, err)

		assert.Equal(t, store.ClearingState{LastProbed: 2}, clearingState(t, db))
	})

	t.Run("anAbandonedSendIsRememberedButNotProbed", func(t *testing.T) {
		chain := newFakeChain()
		chain.send(1, 2)
		chain.prune(1)

		db := watcherStore(t)
		abandoning := newClearer(chain, db, ClearConfig{AbandonUnrecoverablePackets: true})

		result, err := abandoning.Clear(ctx, sourceClientID)
		require.NoError(t, err)

		assert.Equal(t, Result{Probed: 2, Outstanding: 2, Recovered: 1, Abandoned: 1}, result)
		assert.Equal(t, []uint64{2}, recorded(t, db))
		assert.Equal(t, store.ClearingState{LastProbed: 2, Unresolved: []uint64{1}}, clearingState(t, db))

		before := len(chain.probeCalls())

		result, err = abandoning.Clear(ctx, sourceClientID)
		require.NoError(t, err)

		// on record but out of the probe, so it costs the pass nothing
		assert.Empty(t, chain.probeCalls()[before:])
		assert.Equal(t, 1, result.Abandoned)
		assert.Equal(t, store.ClearingState{LastProbed: 2, Unresolved: []uint64{1}}, clearingState(t, db))

		// an archive endpoint turns up later: turning the setting off is the
		// whole recovery, since the sequence was never forgotten
		chain.unprune(1)

		result, err = newTestClearer(chain, db).Clear(ctx, sourceClientID)
		require.NoError(t, err)

		assert.Equal(t, [][]uint64{{1}}, chain.probeCalls()[before:])
		assert.Equal(t, Result{Probed: 1, Outstanding: 1, Recovered: 1}, result)
		assert.Equal(t, []uint64{1, 2}, recorded(t, db))
		assert.Equal(t, store.ClearingState{LastProbed: 2}, clearingState(t, db))
	})

	t.Run("aFailedRowWriteLeavesNoWatermark", func(t *testing.T) {
		chain := newFakeChain()
		chain.send(1, 2)

		db := watcherStore(t)
		storage := failingWrites{ClearStore: db, err: errors.New("disk is full")}

		_, err := newTestClearer(chain, storage).Clear(ctx, sourceClientID)
		require.ErrorContains(t, err, "disk is full")

		assert.Empty(t, recorded(t, db))
		assert.Equal(t, store.ClearingState{}, clearingState(t, db))
	})

	t.Run("everyProbeNamesTheHeadTheSequenceWasReadAt", func(t *testing.T) {
		chain := newFakeChain()
		chain.mine(7)
		chain.send(1, 2)

		_, err := newTestClearer(chain, watcherStore(t)).Clear(ctx, sourceClientID)
		require.NoError(t, err)

		assert.Equal(t, []uint64{7}, chain.probeHeights())
	})

	t.Run("theHeightMovesUpWithTheChainBetweenChunks", func(t *testing.T) {
		chain := newFakeChain()
		chain.mine(10)

		for sequence := uint64(1); sequence <= probeChunk+1; sequence++ {
			chain.send(sequence)
		}

		// a cold pass outlives the state a non-archive node keeps, so the
		// second chunk reads at wherever the chain has got to by then
		chain.serveHeads(10, 10, 12)

		_, err := newTestClearer(chain, watcherStore(t)).Clear(ctx, sourceClientID)
		require.NoError(t, err)

		assert.Equal(t, []uint64{10, 12}, chain.probeHeights())
	})

	t.Run("aHeadReadFromALaggingNodeCannotDragTheProbeBack", func(t *testing.T) {
		chain := newFakeChain()
		chain.mine(10)

		const sent = probeChunk + 1

		for sequence := uint64(1); sequence <= sent; sequence++ {
			chain.send(sequence)
		}

		// the pass opens at 10, then the refresh lands on a node six blocks behind
		chain.serveHeads(10, 4)

		result, err := newTestClearer(chain, watcherStore(t)).Clear(ctx, sourceClientID)
		require.NoError(t, err)

		assert.Equal(t, []uint64{10, 10}, chain.probeHeights())
		// probing at 4 would have read the second chunk as settled and written it off
		assert.Equal(t, sent, result.Recovered)
	})

	t.Run("aFailedHeaderReadAbortsThePass", func(t *testing.T) {
		chain := newFakeChain()
		chain.send(1, 2)
		chain.failHead(errors.New("rpc timed out"))

		db := watcherStore(t)

		_, err := newTestClearer(chain, db).Clear(ctx, sourceClientID)
		require.ErrorContains(t, err, "rpc timed out")

		assert.Empty(t, chain.probeCalls())
		assert.Empty(t, recorded(t, db))
		assert.Equal(t, store.ClearingState{}, clearingState(t, db))
	})
}

// A sequence unresolved before and after must produce neither half of the
// delta: adding would reset the first_seen_at an operator reads to tell how
// long a packet has been stuck, and resolving would forget it outright.
func TestUnresolvedDelta(t *testing.T) {
	for name, tt := range map[string]struct {
		probed, unresolved []uint64
		add, resolve       []uint64
	}{
		"unchanged":   {probed: []uint64{5, 9}, unresolved: []uint64{5, 9}},
		"add":         {probed: []uint64{5}, unresolved: []uint64{5, 9}, add: []uint64{9}},
		"resolve":     {probed: []uint64{5, 9}, unresolved: []uint64{9}, resolve: []uint64{5}},
		"replaced":    {probed: []uint64{5}, unresolved: []uint64{9}, add: []uint64{9}, resolve: []uint64{5}},
		"allResolved": {probed: []uint64{5, 9}, resolve: []uint64{5, 9}},
		// a pass that probed nothing held, which is what abandoning does, must
		// resolve nothing: it learned nothing about the set it carried
		"nothingProbed": {unresolved: []uint64{9}, add: []uint64{9}},
		"bothEmpty":     {},
	} {
		t.Run(name, func(t *testing.T) {
			delta := unresolvedDelta(tt.probed, tt.unresolved, 42)
			assert.Equal(t, tt.add, delta.Add)
			assert.Equal(t, tt.resolve, delta.Resolve)
			assert.Equal(t, uint64(42), delta.Height)
		})
	}
}

// assertCovers checks the calls split at size and cover 1..sent between them.
func assertCovers(t *testing.T, calls [][]uint64, size, sent int) {
	t.Helper()

	var covered []uint64

	for _, call := range calls {
		assert.LessOrEqual(t, len(call), size)
		covered = append(covered, call...)
	}

	require.Greater(t, len(calls), 1)
	assert.Equal(t, sequenceRange(1, uint64(sent)), covered)
}
