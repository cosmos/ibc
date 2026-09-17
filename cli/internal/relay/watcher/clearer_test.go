// SPDX-License-Identifier: Apache-2.0

package watcher

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/internal/store"
)

// waitFor bounds how long a test blocks on a clearing pass's own goroutine.
const waitFor = 5 * time.Second

func newTestClearer(chain Chain, storage ClearStore) *Clearer {
	return newClearer(chain, storage, Config{})
}

func newClearer(chain Chain, storage ClearStore, cfg Config) *Clearer {
	return NewClearer(sourceChainID, testConnections(), chain, storage, cfg, slog.Default())
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
		chain := newFakeChain(t)
		chain.sendSequences(1, 2, 3, 4, 5)
		chain.settleSequences(1, 2, 4)

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
		chain := newFakeChain(t)
		chain.sendSequences(1, 2, 3)
		chain.settleSequences(1, 2, 3)

		db := watcherStore(t)

		result, err := newTestClearer(chain, db).Clear(ctx, sourceClientID)
		require.NoError(t, err)

		assert.Equal(t, Result{Probed: 3}, result)
		assert.Empty(t, recorded(t, db))
		assert.Empty(t, chain.findCalls())
	})

	t.Run("aForeignSendIsResolvedWithoutARow", func(t *testing.T) {
		chain := newFakeChain(t)
		chain.sendSequences(1, 2)
		chain.misrouteSequences(1)

		db := watcherStore(t)
		clearer := newTestClearer(chain, db)

		result, err := clearer.Clear(ctx, sourceClientID)
		require.NoError(t, err)

		// the endpoint served the send, so declining to record it resolves it:
		// carrying it would re-probe a packet no pass can ever relay
		assert.Equal(t, Result{Probed: 2, Outstanding: 2, Recovered: 1}, result)
		assert.Equal(t, []uint64{2}, recorded(t, db))
		assert.Equal(t, store.ClearingState{LastProbed: 2}, clearingState(t, db))

		probes, finds := len(chain.probeCalls()), len(chain.findCalls())

		_, err = clearer.Clear(ctx, sourceClientID)
		require.NoError(t, err)

		// the watermark covers it, so the next pass neither probes nor looks it up
		assert.Empty(t, chain.probeCalls()[probes:])
		assert.Empty(t, chain.findCalls()[finds:])
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
				chain := newFakeChain(t)
				chain.sendSequences(1, 2)

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
		chain := newFakeChain(t)
		chain.sendSequences(1, 2, 3, 4)

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
		chain := newFakeChain(t)
		chain.sendSequences(1, 2)

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
		chain := newFakeChain(t)
		chain.sendSequences(1, 2, 3)

		db := watcherStore(t)
		require.NoError(t, db.SetClearingState(ctx, sourceChainID, sourceClientID, 9, store.UnresolvedDelta{}))

		_, err := newTestClearer(chain, db).Clear(ctx, sourceClientID)
		require.NoError(t, err)

		assert.Equal(t, uint64(9), clearingState(t, db).LastProbed)
	})

	t.Run("aClientWithNothingSentProbesNothing", func(t *testing.T) {
		chain := newFakeChain(t)

		result, err := newTestClearer(chain, watcherStore(t)).Clear(ctx, sourceClientID)
		require.NoError(t, err)

		assert.Equal(t, Result{}, result)
		assert.Empty(t, chain.probeCalls())
	})

	t.Run("probeFailureWritesNothing", func(t *testing.T) {
		chain := newFakeChain(t)
		chain.sendSequences(1, 2)
		chain.failProbe(errors.New("rpc refused the batch"))

		db := watcherStore(t)

		_, err := newTestClearer(chain, db).Clear(ctx, sourceClientID)
		require.ErrorContains(t, err, "rpc refused the batch")
		assert.Empty(t, recorded(t, db))
		assert.Equal(t, store.ClearingState{}, clearingState(t, db))
	})

	t.Run("bothQueriesAreChunked", func(t *testing.T) {
		chain := newFakeChain(t)

		const sent = probeChunk + sendLogChunk

		for sequence := uint64(1); sequence <= sent; sequence++ {
			chain.sendSequences(sequence)
		}

		result, err := newTestClearer(chain, watcherStore(t)).Clear(ctx, sourceClientID)
		require.NoError(t, err)

		assert.Equal(t, sent, result.Probed)
		assert.Equal(t, sent, result.Recovered)

		assertCovers(t, chain.probeCalls(), probeChunk, sent)
		assertCovers(t, chain.findCalls(), sendLogChunk, sent)
	})

	t.Run("theSequenceIsReadBeforeTheProbe", func(t *testing.T) {
		chain := newFakeChain(t)
		chain.sendSequences(1, 2)

		release := chain.blockClears()
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
		chain := newFakeChain(t)

		const sent = 2 * probeChunk

		for sequence := uint64(1); sequence <= sent; sequence++ {
			chain.sendSequences(sequence)
		}

		// everything is settled but one packet, which stays stuck in a terminal
		// state: a client's history must not pin the probe to it
		chain.settleSequences(sequenceRange(2, sent)...)

		clearer := newTestClearer(chain, watcherStore(t))

		cold, err := clearer.Clear(ctx, sourceClientID)
		require.NoError(t, err)
		assert.Equal(t, sent, cold.Probed)

		coldCalls := len(chain.probeCalls())
		require.Equal(t, 2, coldCalls)

		chain.sendSequences(sent + 1)

		warm, err := clearer.Clear(ctx, sourceClientID)
		require.NoError(t, err)

		assert.Equal(t, 1, warm.Probed)
		assert.Equal(t, [][]uint64{{sent + 1}}, chain.probeCalls()[coldCalls:])
		assert.Less(t, len(chain.probeCalls())-coldCalls, coldCalls)
	})

	t.Run("anUnresolvedSendIsProbedUntilItSettles", func(t *testing.T) {
		chain := newFakeChain(t)
		chain.sendSequences(1, 2)
		chain.pruneSequences(1)

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
		chain.settleSequences(1)

		result, err = clearer.Clear(ctx, sourceClientID)
		require.NoError(t, err)

		assert.Zero(t, result.Unresolved)
		assert.Equal(t, store.ClearingState{LastProbed: 2}, clearingState(t, db))
	})

	t.Run("aLaggingHeadCannotResolveAnUnresolvedSend", func(t *testing.T) {
		chain := newFakeChain(t)
		chain.mineBlocks(10)
		chain.sendSequences(1, 2)
		chain.pruneSequences(1)

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
		chain.settleSequences(1)

		_, err = clearer.Clear(ctx, sourceClientID)
		require.NoError(t, err)

		assert.Equal(t, store.ClearingState{LastProbed: 2}, clearingState(t, db))
	})

	// a mixed pass probes a new range as well as the carried set, and the chain
	// moves on in between. The carried sequence has to be weighed against the
	// height it was read at, not the one a later chunk raised the pass to
	t.Run("aHeightRaisedByANewRangeCannotResolveTheCarriedSet", func(t *testing.T) {
		chain := newFakeChain(t)

		// the new sequence is visible at the head the pass opens on, the carried
		// one only from the height it was last seen live at
		chain.mineBlocks(10)
		chain.sendSequences(3)
		chain.mineBlocks(10)
		chain.sendSequences(1)
		chain.pruneSequences(1)

		db := watcherStore(t)
		require.NoError(t, db.SetClearingState(ctx, sourceChainID, sourceClientID, 2, store.UnresolvedDelta{
			Add:    []uint64{1},
			Height: 20,
		}))

		// the pass opens on a node at 10, where sequence 1 reads absent, and the
		// chain has reached 30 by the time the probe refreshes its height
		chain.serveHeads(10, 10, 30)

		result, err := newTestClearer(chain, db).Clear(ctx, sourceClientID)
		require.NoError(t, err)

		// resolving on the opening height would have forgotten the only record of
		// a live packet the raised height was never read against
		assert.Equal(t, Result{Probed: 2, Outstanding: 2, Recovered: 1, Unresolved: 1}, result)
		assert.Equal(t, []uint64{3}, recorded(t, db))
		assert.Equal(t, store.ClearingState{LastProbed: 3, Unresolved: []uint64{1}}, clearingState(t, db))
		assert.Equal(t, [][]uint64{{3}, {1}}, chain.probeCalls())
		assert.Equal(t, []uint64{10, 30}, chain.probeHeights())
	})

	t.Run("anAbandonedSendIsRememberedButNotProbed", func(t *testing.T) {
		chain := newFakeChain(t)
		chain.sendSequences(1, 2)
		chain.pruneSequences(1)

		db := watcherStore(t)
		abandoning := newClearer(chain, db, Config{AbandonUnrecoverablePackets: true})

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
		chain.unpruneSequences(1)

		result, err = newTestClearer(chain, db).Clear(ctx, sourceClientID)
		require.NoError(t, err)

		assert.Equal(t, [][]uint64{{1}}, chain.probeCalls()[before:])
		assert.Equal(t, Result{Probed: 1, Outstanding: 1, Recovered: 1}, result)
		assert.Equal(t, []uint64{1, 2}, recorded(t, db))
		assert.Equal(t, store.ClearingState{LastProbed: 2}, clearingState(t, db))
	})

	t.Run("aFailedRowWriteLeavesNoWatermark", func(t *testing.T) {
		chain := newFakeChain(t)
		chain.sendSequences(1, 2)

		db := watcherStore(t)
		storage := failingWrites{ClearStore: db, err: errors.New("disk is full")}

		_, err := newTestClearer(chain, storage).Clear(ctx, sourceClientID)
		require.ErrorContains(t, err, "disk is full")

		assert.Empty(t, recorded(t, db))
		assert.Equal(t, store.ClearingState{}, clearingState(t, db))
	})

	t.Run("everyProbeNamesTheHeadTheSequenceWasReadAt", func(t *testing.T) {
		chain := newFakeChain(t)
		chain.mineBlocks(7)
		chain.sendSequences(1, 2)

		_, err := newTestClearer(chain, watcherStore(t)).Clear(ctx, sourceClientID)
		require.NoError(t, err)

		assert.Equal(t, []uint64{7}, chain.probeHeights())
	})

	t.Run("aCommitmentSettledAboveTheProbeHeightReadsLive", func(t *testing.T) {
		chain := newFakeChain(t)
		chain.mineBlocks(5)
		chain.sendSequences(1)
		chain.mineBlocks(10)
		chain.settleSequences(1)

		// the pass reads at 5, where the ack that deleted the commitment at 15
		// has not landed: deletion only counts from the height it happened at
		chain.serveHeads(5, 5)

		db := watcherStore(t)

		result, err := newTestClearer(chain, db).Clear(ctx, sourceClientID)
		require.NoError(t, err)

		assert.Equal(t, []uint64{5}, chain.probeHeights())
		assert.Equal(t, Result{Probed: 1, Outstanding: 1, Recovered: 1}, result)
		assert.Equal(t, []uint64{1}, recorded(t, db))
	})

	t.Run("theHeightMovesUpWithTheChainBetweenChunks", func(t *testing.T) {
		chain := newFakeChain(t)
		chain.mineBlocks(10)

		for sequence := uint64(1); sequence <= probeChunk+1; sequence++ {
			chain.sendSequences(sequence)
		}

		// a cold pass outlives the state a non-archive node keeps, so the
		// second chunk reads at wherever the chain has got to by then
		chain.serveHeads(10, 10, 12)

		_, err := newTestClearer(chain, watcherStore(t)).Clear(ctx, sourceClientID)
		require.NoError(t, err)

		assert.Equal(t, []uint64{10, 12}, chain.probeHeights())
	})

	t.Run("aHeadReadFromALaggingNodeCannotDragTheProbeBack", func(t *testing.T) {
		chain := newFakeChain(t)
		chain.mineBlocks(10)

		const sent = probeChunk + 1

		for sequence := uint64(1); sequence <= sent; sequence++ {
			chain.sendSequences(sequence)
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
		chain := newFakeChain(t)
		chain.sendSequences(1, 2)
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
