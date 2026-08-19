// SPDX-License-Identifier: Apache-2.0

package watcher

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/store"
	"github.com/cosmos/ibc/link/internal/tests/mocks"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

const (
	sourceChainID  = "1"
	sourceClientID = "base-0"
	destChainID    = "8453"
	destClientID   = "ethereum-0"
	sendTxHash     = "0x60016c34c02278856c81a41ce857ac4bb837a2f4a13c95207e08cbc9e8f2b706"

	// waitFor bounds how long a test blocks on the watcher's own goroutine.
	waitFor = 5 * time.Second
)

var blockTime = time.Unix(1_700_000_000, 0).UTC()

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

func newTestWatcher(t *testing.T, subscriber Subscriber, storage PacketStore) *Watcher {
	t.Helper()

	return New(sourceChainID, testConnections(), subscriber, storage, slog.Default())
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

func watcherStore(t *testing.T) *store.SqliteDB {
	t.Helper()

	db, err := store.NewSqliteInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.MigrateUp()
	require.NoError(t, err)

	return db
}

// subscriber scripts a MockSubscriber so that every subscribe hands the test a
// live subscription. The watcher opens one per reconnect, so they queue up in
// the order it opened them.
type subscriber struct {
	*mocks.MockSubscriber

	t *testing.T

	mu        sync.Mutex
	clientIDs []string

	opened chan *subscription
}

func newSubscriber(t *testing.T) *subscriber {
	t.Helper()

	s := &subscriber{
		MockSubscriber: mocks.NewMockSubscriber(t),
		t:              t,
		opened:         make(chan *subscription, 8),
	}

	s.EXPECT().
		SubscribeSendPackets(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(
			ctx context.Context,
			clientIDs []string,
			out chan<- v2.PacketEvent,
		) (v2.Subscription, error) {
			s.mu.Lock()
			s.clientIDs = clientIDs
			s.mu.Unlock()

			return s.open(ctx, out), nil
		}).
		Maybe()

	return s
}

func (s *subscriber) open(ctx context.Context, out chan<- v2.PacketEvent) v2.Subscription {
	opened := &subscription{
		ctx:      ctx,
		out:      out,
		errs:     make(chan error, 1),
		released: make(chan struct{}),
	}

	// stands in for the real subscription's goroutine: it lives until its
	// context is canceled, so a leaked context leaves released open
	go func() {
		<-ctx.Done()
		close(opened.released)
	}()

	opened.mock = mocks.NewMockSubscription(s.t)
	opened.mock.EXPECT().Err().Return(opened.errs).Maybe()
	opened.mock.EXPECT().Unsubscribe().Return().Maybe()

	s.opened <- opened

	return opened.mock
}

func (s *subscriber) watchedClientIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.clientIDs
}

// next blocks until the watcher opens its next subscription.
func (s *subscriber) next(t *testing.T) *subscription {
	t.Helper()

	select {
	case sub := <-s.opened:
		return sub
	case <-time.After(waitFor):
		t.Fatal("watcher did not open a subscription")

		return nil
	}
}

// subscription is one opened stream, which the test drives as the chain would.
type subscription struct {
	mock     *mocks.MockSubscription
	ctx      context.Context //nolint:containedctx // the test asserts on its cancellation
	out      chan<- v2.PacketEvent
	errs     chan error
	released chan struct{}
}

func (s *subscription) deliver(t *testing.T, event v2.PacketEvent) {
	t.Helper()

	select {
	case s.out <- event:
	case <-time.After(waitFor):
		t.Fatal("watcher did not accept the delivered event")
	}
}

func (s *subscription) assertUnsubscribed(t *testing.T) {
	t.Helper()

	s.mock.AssertCalled(t, "Unsubscribe")
}

// writes reports each row a MockPacketStore is asked to write, so a test can
// wait for the watcher's own goroutine to get there.
func writes(t *testing.T, err error) (*mocks.MockPacketStore, <-chan store.UpsertPacket) {
	t.Helper()

	storage := mocks.NewMockPacketStore(t)
	written := make(chan store.UpsertPacket, 8)

	storage.EXPECT().
		UpsertPacket(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, input store.UpsertPacket) error {
			written <- input

			return err
		}).
		Maybe()

	return storage, written
}

func awaitWrite(t *testing.T, written <-chan store.UpsertPacket) store.UpsertPacket {
	t.Helper()

	select {
	case input := <-written:
		return input
	case <-time.After(waitFor):
		t.Fatal("watcher did not write the packet")

		return store.UpsertPacket{}
	}
}

func TestWatcherHandleEvent(t *testing.T) {
	ctx := context.Background()

	t.Run("sendPacketWritesOneRow", func(t *testing.T) {
		db := watcherStore(t)
		w := newTestWatcher(t, newSubscriber(t), db)

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

	// the mock store has no expectation for a write, so reaching one fails the test
	t.Run("reorgedOutEventWritesNothing", func(t *testing.T) {
		w := newTestWatcher(t, newSubscriber(t), mocks.NewMockPacketStore(t))

		event := sendPacketEvent(7)
		event.Removed = true

		require.NoError(t, w.HandleEvent(ctx, event))
	})

	t.Run("otherEventKindsWriteNothing", func(t *testing.T) {
		w := newTestWatcher(t, newSubscriber(t), mocks.NewMockPacketStore(t))

		event := sendPacketEvent(7)
		event.Kind = v2.KindWriteAck

		require.NoError(t, w.HandleEvent(ctx, event))
	})
}

func TestWatcherLoop(t *testing.T) {
	t.Run("subscribesToTheConfiguredClients", func(t *testing.T) {
		chain := newSubscriber(t)
		w := newTestWatcher(t, chain, mocks.NewMockPacketStore(t))

		require.NoError(t, w.Start())
		t.Cleanup(func() { require.NoError(t, w.Stop()) })

		chain.next(t)
		assert.Equal(t, []string{sourceClientID}, chain.watchedClientIDs())
	})

	t.Run("storeErrorDoesNotKillTheLoop", func(t *testing.T) {
		storage, written := writes(t, errors.New("store unavailable"))
		chain := newSubscriber(t)
		w := newTestWatcher(t, chain, storage)

		require.NoError(t, w.Start())
		t.Cleanup(func() { require.NoError(t, w.Stop()) })

		sub := chain.next(t)
		sub.deliver(t, sendPacketEvent(1))
		sub.deliver(t, sendPacketEvent(2))

		assert.Equal(t, uint64(1), awaitWrite(t, written).PacketSequenceNumber)
		assert.Equal(t, uint64(2), awaitWrite(t, written).PacketSequenceNumber)
	})

	t.Run("stopBlocksUntilTheLoopExits", func(t *testing.T) {
		chain := newSubscriber(t)
		w := newTestWatcher(t, chain, mocks.NewMockPacketStore(t))

		require.NoError(t, w.Start())
		sub := chain.next(t)

		require.NoError(t, w.Stop())

		select {
		case <-w.stopped:
		default:
			t.Fatal("Stop returned before the loop exited")
		}

		sub.assertUnsubscribed(t)
		require.Error(t, sub.ctx.Err())
	})

	t.Run("stopBeforeStartIsANoop", func(t *testing.T) {
		require.NoError(t, newTestWatcher(t, newSubscriber(t), mocks.NewMockPacketStore(t)).Stop())
	})
}
