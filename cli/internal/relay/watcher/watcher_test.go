// SPDX-License-Identifier: Apache-2.0

package watcher

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	"github.com/cosmos/ibc/cli/internal/config"
	"github.com/cosmos/ibc/cli/internal/store"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

const (
	sourceChainID  = "1"
	sourceClientID = "base-0"
	destChainID    = "8453"
	destClientID   = "ethereum-0"
	sendTxHash     = "0x60016c34c02278856c81a41ce857ac4bb837a2f4a13c95207e08cbc9e8f2b706"
)

var blockTime = time.Unix(1_700_000_000, 0).UTC()

// subscriber stands in for the chain-side event stream. It opens a fresh
// subscription per subscribe, so a test can watch the watcher reconnect, and
// failNext makes the next subscribe fail instead.
type subscriber struct {
	mu       sync.Mutex
	subs     []*subscription
	failWith error
}

func newSubscriber() *subscriber { return &subscriber{} }

func (s *subscriber) SubscribeSendPackets(
	ctx context.Context,
	clientIDs []string,
	out chan<- v2.PacketEvent,
) (v2.Subscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.failWith; err != nil {
		s.failWith = nil

		return nil, err
	}

	sub := &subscription{clientIDs: clientIDs, ctx: ctx, out: out, errs: make(chan error, 1)}
	s.subs = append(s.subs, sub)

	return sub, nil
}

// failNext makes the next subscribe fail rather than open.
func (s *subscriber) failNext(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.failWith = err
}

func (s *subscriber) opened() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.subs)
}

// latest is the subscription the watcher is currently reading from.
func (s *subscriber) latest(t *testing.T) *subscription {
	t.Helper()

	s.mu.Lock()
	defer s.mu.Unlock()

	require.NotEmpty(t, s.subs, "watcher has not subscribed")

	return s.subs[len(s.subs)-1]
}

// subscription is one opened stream, which the test drives as the chain would.
type subscription struct {
	clientIDs    []string
	ctx          context.Context //nolint:containedctx // the test asserts on its cancellation
	out          chan<- v2.PacketEvent
	errs         chan error
	unsubscribed bool
}

func (s *subscription) Err() <-chan error { return s.errs }

func (s *subscription) Unsubscribe() { s.unsubscribed = true }

// failingStore fails the watcher's own row writes, leaving the reads intact.
type failingStore struct {
	ClearStore

	err error
}

func (s *failingStore) UpsertPacket(ctx context.Context, input store.UpsertPacket) error {
	if s.err != nil {
		return s.err
	}

	return s.ClearStore.UpsertPacket(ctx, input)
}

func testConnections() []config.ConnectionConfig {
	return []config.ConnectionConfig{{
		Alias: "test-connection",
		ClientA: config.ClientEnd{
			ChainID:  sourceChainID,
			ClientID: sourceClientID,
			Type:     config.ClientTypeAttestation,
		},
		ClientB: config.ClientEnd{
			ChainID:  destChainID,
			ClientID: destClientID,
			Type:     config.ClientTypeAttestation,
		},
	}}
}

// newTestWatcher builds a watcher whose clearing pass only ever runs on a
// reconnect, so a test that says nothing about clearing gets none.
func newTestWatcher(chain Subscriber, storage ClearStore) *Watcher {
	return newClearingWatcher(chain, newFakeChain(), storage, ClearConfig{Interval: time.Hour})
}

func newClearingWatcher(
	chain Subscriber,
	querier OutstandingQuerier,
	storage ClearStore,
	clearing ClearConfig,
) *Watcher {
	return New(
		sourceChainID,
		testConnections(),
		chain,
		querier,
		storage,
		clearing,
		DefaultMinBackoff,
		DefaultMaxBackoff,
		slog.Default(),
	)
}

func sendPacketEvent(sequence uint64) v2.PacketEvent {
	return v2.PacketEvent{
		Height:    100,
		BlockTime: blockTime,
		Kind:      v2.KindSendPacket,
		TxHash:    sendTxHash,
		Packet: channeltypesv2.Packet{
			Sequence:          sequence,
			SourceClient:      sourceClientID,
			DestinationClient: destClientID,
			TimeoutTimestamp:  uint64(blockTime.Add(time.Hour).Unix()),
		},
	}
}

func TestWatcherHandleEvent(t *testing.T) {
	ctx := context.Background()

	t.Run("sendPacketWritesOneRow", func(t *testing.T) {
		db := watcherStore(t)
		w := newTestWatcher(newSubscriber(), db)

		require.NoError(t, w.HandleEvent(ctx, sendPacketEvent(7)))

		packets, err := db.ListPacketsBySourceTx(ctx, sourceChainID, sendTxHash)
		require.NoError(t, err)
		require.Len(t, packets, 1)

		packet := packets[0]
		assert.Equal(t, store.RelayStatusPending, packet.Status)
		assert.Equal(t, uint64(7), packet.PacketSequenceNumber)
		assert.Equal(t, sourceChainID, packet.SourceChainID)
		assert.Equal(t, destChainID, packet.DestinationChainID)
		assert.Equal(t, sourceClientID, packet.PacketSourceClientID)
		assert.Equal(t, destClientID, packet.PacketDestinationClientID)
		assert.Equal(t, sendTxHash, packet.SourceTxHash)
		assert.Equal(t, blockTime, packet.SourceTxTime.UTC())
		assert.Equal(t, blockTime.Add(time.Hour), packet.PacketTimeoutTimestamp.UTC())
	})

	t.Run("reorgedOutEventWritesNothing", func(t *testing.T) {
		db := watcherStore(t)
		w := newTestWatcher(newSubscriber(), db)

		event := sendPacketEvent(7)
		event.Removed = true

		require.NoError(t, w.HandleEvent(ctx, event))
		assert.Empty(t, recorded(t, db))
	})

	t.Run("anotherDestinationClientWritesNothing", func(t *testing.T) {
		db := watcherStore(t)
		w := newTestWatcher(newSubscriber(), db)

		event := sendPacketEvent(7)
		event.Packet.DestinationClient = "ethereum-9"

		require.NoError(t, w.HandleEvent(ctx, event))
		assert.Empty(t, recorded(t, db))
	})

	t.Run("anUnconfiguredSourceClientWritesNothing", func(t *testing.T) {
		db := watcherStore(t)
		w := newTestWatcher(newSubscriber(), db)

		event := sendPacketEvent(7)
		event.Packet.SourceClient = "base-9"

		require.NoError(t, w.HandleEvent(ctx, event))
		assert.Empty(t, recorded(t, db))
	})

	t.Run("otherEventKindsWriteNothing", func(t *testing.T) {
		db := watcherStore(t)
		w := newTestWatcher(newSubscriber(), db)

		event := sendPacketEvent(7)
		event.Kind = v2.KindWriteAck

		require.NoError(t, w.HandleEvent(ctx, event))
		assert.Empty(t, recorded(t, db))
	})
}

// start runs a watcher up to its first open subscription.
func start(t *testing.T, w *Watcher) *Watcher {
	t.Helper()

	require.NoError(t, w.Start())
	synctest.Wait()

	return w
}

// TestWatcherStart runs the loop inside a synctest bubble: Wait returns once
// the watcher's goroutine is blocked again and sleeping advances the backoff
// timers instantly, so nothing here has to poll or wait on real time.
func TestWatcherStart(t *testing.T) {
	t.Run("subscribesToTheConfiguredClients", func(t *testing.T) {
		db := watcherStore(t)

		synctest.Test(t, func(t *testing.T) {
			chain := newSubscriber()
			w := start(t, newTestWatcher(chain, db))

			assert.Equal(t, []string{sourceClientID}, chain.latest(t).clientIDs)
			require.NoError(t, w.Stop())
		})
	})

	t.Run("storeErrorDoesNotKillTheLoop", func(t *testing.T) {
		db := watcherStore(t)

		synctest.Test(t, func(t *testing.T) {
			chain := newSubscriber()
			storage := &failingStore{ClearStore: db, err: errors.New("store unavailable")}
			w := start(t, newTestWatcher(chain, storage))

			chain.latest(t).out <- sendPacketEvent(1)
			chain.latest(t).out <- sendPacketEvent(2)
			synctest.Wait()

			require.Empty(t, recorded(t, db))

			// the loop is still reading events, which is what the failures
			// must not have cost it
			storage.err = nil
			chain.latest(t).out <- sendPacketEvent(3)
			synctest.Wait()

			assert.Equal(t, []uint64{3}, recorded(t, db))
			require.NoError(t, w.Stop())
		})
	})

	t.Run("subscriptionErrorResubscribes", func(t *testing.T) {
		db := watcherStore(t)

		synctest.Test(t, func(t *testing.T) {
			chain := newSubscriber()
			w := start(t, newTestWatcher(chain, db))

			first := chain.latest(t)
			first.errs <- errors.New("websocket closed")

			time.Sleep(DefaultMinBackoff)
			synctest.Wait()

			require.Equal(t, 2, chain.opened())

			// the dropped subscription's context must be dead, or every
			// reconnect leaks the goroutine feeding it
			assert.True(t, first.unsubscribed)
			require.Error(t, first.ctx.Err())

			chain.latest(t).out <- sendPacketEvent(1)
			synctest.Wait()

			assert.Equal(t, []uint64{1}, recorded(t, db))
			require.NoError(t, w.Stop())
		})
	})

	t.Run("resubscribeErrorRetriesWithBackoff", func(t *testing.T) {
		db := watcherStore(t)

		synctest.Test(t, func(t *testing.T) {
			chain := newSubscriber()
			w := start(t, newTestWatcher(chain, db))

			chain.failNext(errors.New("dial failed"))
			chain.latest(t).errs <- errors.New("websocket closed")

			time.Sleep(DefaultMinBackoff)
			synctest.Wait()
			require.Equal(t, 1, chain.opened(), "the retry that failed should not have opened anything")

			// the failed retry doubles the wait before the next one
			time.Sleep(2 * DefaultMinBackoff)
			synctest.Wait()
			require.Equal(t, 2, chain.opened())

			chain.latest(t).out <- sendPacketEvent(1)
			synctest.Wait()

			assert.Equal(t, []uint64{1}, recorded(t, db))
			require.NoError(t, w.Stop())
		})
	})

	t.Run("stopBlocksUntilTheLoopExits", func(t *testing.T) {
		db := watcherStore(t)

		synctest.Test(t, func(t *testing.T) {
			chain := newSubscriber()
			w := start(t, newTestWatcher(chain, db))

			require.NoError(t, w.Stop())

			select {
			case <-w.stopped:
			default:
				t.Fatal("Stop returned before the loop exited")
			}

			// canceling the subscription context is what releases the
			// subscription's goroutine; unsubscribing alone leaves it running
			assert.True(t, chain.latest(t).unsubscribed)
			require.Error(t, chain.latest(t).ctx.Err())
		})
	})

	t.Run("subscribeErrorFailsStart", func(t *testing.T) {
		db := watcherStore(t)

		synctest.Test(t, func(t *testing.T) {
			chain := newSubscriber()
			chain.failNext(errors.New("dial failed"))
			w := newTestWatcher(chain, db)

			require.ErrorContains(t, w.Start(), "subscribing to send packets")

			// a watcher that never started has nothing to stop, and Stop must
			// not block waiting for a loop that was never running
			require.NoError(t, w.Stop())
		})
	})

	t.Run("stopBeforeStartIsANoop", func(t *testing.T) {
		require.NoError(t, newTestWatcher(newSubscriber(), watcherStore(t)).Stop())
	})
}

func TestWatcherClearingLoop(t *testing.T) {
	t.Run("aSubscriptionDropClearsTheGapItLeft", func(t *testing.T) {
		db := watcherStore(t)

		synctest.Test(t, func(t *testing.T) {
			chain := newSubscriber()
			outstanding := newFakeChain()
			w := start(t, newClearingWatcher(chain, outstanding, db, ClearConfig{Interval: time.Hour}))

			chain.latest(t).out <- sendPacketEvent(1)
			synctest.Wait()
			require.Equal(t, []uint64{1}, recorded(t, db))

			// sent while nothing was listening: the subscription cannot have
			// seen these, so only the clearing pass can recover them
			outstanding.send(1, 2, 3)
			chain.latest(t).errs <- errors.New("websocket closed")

			time.Sleep(DefaultMinBackoff)
			synctest.Wait()

			assert.Equal(t, []uint64{1, 2, 3}, recorded(t, db))
			require.NoError(t, w.Stop())
		})
	})

	t.Run("clearOnStartHonoursTheFlag", func(t *testing.T) {
		for _, onStart := range []bool{true, false} {
			t.Run(map[bool]string{true: "enabled", false: "disabled"}[onStart], func(t *testing.T) {
				db := watcherStore(t)

				synctest.Test(t, func(t *testing.T) {
					outstanding := newFakeChain()
					outstanding.send(1)

					clearing := ClearConfig{OnStart: onStart, Interval: time.Hour}
					w := start(t, newClearingWatcher(newSubscriber(), outstanding, db, clearing))

					if onStart {
						assert.Equal(t, []uint64{1}, recorded(t, db))
					} else {
						assert.Empty(t, recorded(t, db))
					}

					require.NoError(t, w.Stop())
				})
			})
		}
	})

	t.Run("theIntervalKeepsClearing", func(t *testing.T) {
		db := watcherStore(t)

		synctest.Test(t, func(t *testing.T) {
			outstanding := newFakeChain()
			outstanding.send(1)

			clearing := ClearConfig{Interval: time.Minute}
			w := start(t, newClearingWatcher(newSubscriber(), outstanding, db, clearing))

			require.Empty(t, recorded(t, db))

			time.Sleep(clearing.Interval)
			synctest.Wait()

			assert.Equal(t, []uint64{1}, recorded(t, db))
			require.NoError(t, w.Stop())
		})
	})

	t.Run("rapidReconnectsCoalesceIntoOnePendingPass", func(t *testing.T) {
		db := watcherStore(t)

		synctest.Test(t, func(t *testing.T) {
			chain := newSubscriber()
			outstanding := newFakeChain()
			w := start(t, newClearingWatcher(chain, outstanding, db, ClearConfig{Interval: time.Hour}))

			release := outstanding.hold()

			chain.latest(t).errs <- errors.New("websocket closed")
			time.Sleep(DefaultMinBackoff)
			synctest.Wait()

			require.Equal(t, 1, outstanding.passCount())

			// both reconnects land while that pass is still blocked on the gate
			for range 2 {
				chain.latest(t).errs <- errors.New("websocket closed")
				time.Sleep(DefaultMaxBackoff)
				synctest.Wait()
			}

			release()
			synctest.Wait()

			assert.Equal(t, 2, outstanding.passCount())
			require.NoError(t, w.Stop())
		})
	})

	t.Run("aFailingPassDoesNotStopTheWatcher", func(t *testing.T) {
		db := watcherStore(t)

		synctest.Test(t, func(t *testing.T) {
			chain := newSubscriber()
			outstanding := newFakeChain()
			outstanding.failLatest(errors.New("storage layout moved"))

			clearing := ClearConfig{OnStart: true, Interval: time.Hour}
			w := start(t, newClearingWatcher(chain, outstanding, db, clearing))

			require.Equal(t, 1, outstanding.passCount())

			chain.latest(t).out <- sendPacketEvent(1)
			synctest.Wait()

			assert.Equal(t, []uint64{1}, recorded(t, db))
			require.NoError(t, w.Stop())
		})
	})

	t.Run("stopBlocksUntilTheClearingPassEnds", func(t *testing.T) {
		db := watcherStore(t)

		synctest.Test(t, func(t *testing.T) {
			outstanding := newFakeChain()
			release := outstanding.hold()

			clearing := ClearConfig{OnStart: true, Interval: time.Hour}
			w := start(t, newClearingWatcher(newSubscriber(), outstanding, db, clearing))

			require.Equal(t, 1, outstanding.passCount())

			stopped := make(chan error, 1)
			go func() { stopped <- w.Stop() }()

			synctest.Wait()
			require.Empty(t, stopped, "Stop returned while a clearing pass was still running")

			release()
			synctest.Wait()

			require.NoError(t, <-stopped)
		})
	})
}
