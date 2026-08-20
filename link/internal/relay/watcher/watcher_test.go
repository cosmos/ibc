// SPDX-License-Identifier: Apache-2.0

package watcher

import (
	"context"
	"log/slog"
	"testing"
	"testing/synctest"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/store"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

const (
	sourceChainID  = "1"
	sourceClientID = "base-0"
	destChainID    = "8453"
	destClientID   = "ethereum-0"
	sendTxHash     = "0x60016c34c02278856c81a41ce857ac4bb837a2f4a13c95207e08cbc9e8f2b706"
)

var blockTime = time.Unix(1_700_000_000, 0).UTC()

// chain is both the Subscriber and the single subscription it hands out, so a
// test can act as the chain: deliver events on out, fail the stream on errs.
type chain struct {
	failWith     error
	clientIDs    []string
	ctx          context.Context //nolint:containedctx // the test asserts on its cancellation
	out          chan<- v2.PacketEvent
	errs         chan error
	unsubscribed bool
}

func newChain() *chain {
	return &chain{errs: make(chan error, 1)}
}

func (c *chain) SubscribeSendPackets(
	ctx context.Context,
	clientIDs []string,
	out chan<- v2.PacketEvent,
) (v2.Subscription, error) {
	if err := c.failWith; err != nil {
		c.failWith = nil

		return nil, err
	}

	c.clientIDs, c.ctx, c.out = clientIDs, ctx, out

	return c, nil
}

// failNext makes the next subscribe fail rather than open.
func (c *chain) failNext(err error) { c.failWith = err }

func (c *chain) Err() <-chan error { return c.errs }

func (c *chain) Unsubscribe() { c.unsubscribed = true }

// packetStore records what the watcher writes and optionally fails the write.
type packetStore struct {
	err     error
	written []store.UpsertPacket
}

func newPacketStore(err error) *packetStore {
	return &packetStore{err: err}
}

func (s *packetStore) UpsertPacket(_ context.Context, input store.UpsertPacket) error {
	s.written = append(s.written, input)

	return s.err
}

func (s *packetStore) sequences() []uint64 {
	sequences := make([]uint64, 0, len(s.written))
	for _, packet := range s.written {
		sequences = append(sequences, packet.PacketSequenceNumber)
	}

	return sequences
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

func newTestWatcher(subscriber Subscriber, storage PacketStore) *Watcher {
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

func TestWatcherHandleEvent(t *testing.T) {
	ctx := context.Background()

	t.Run("sendPacketWritesOneRow", func(t *testing.T) {
		storage := newPacketStore(nil)
		w := newTestWatcher(newChain(), storage)

		require.NoError(t, w.HandleEvent(ctx, sendPacketEvent(7)))

		require.Len(t, storage.written, 1)
		assert.Equal(t, store.UpsertPacket{
			Status:                    store.RelayStatusPending,
			SourceChainID:             sourceChainID,
			DestinationChainID:        destChainID,
			SourceTxHash:              sendTxHash,
			SourceTxTime:              blockTime,
			PacketSequenceNumber:      7,
			PacketSourceClientID:      sourceClientID,
			PacketDestinationClientID: destClientID,
			PacketTimeoutTimestamp:    blockTime.Add(time.Hour),
		}, storage.written[0])
	})

	t.Run("reorgedOutEventWritesNothing", func(t *testing.T) {
		storage := newPacketStore(nil)
		w := newTestWatcher(newChain(), storage)

		event := sendPacketEvent(7)
		event.Removed = true

		require.NoError(t, w.HandleEvent(ctx, event))
		assert.Empty(t, storage.written)
	})

	t.Run("otherEventKindsWriteNothing", func(t *testing.T) {
		storage := newPacketStore(nil)
		w := newTestWatcher(newChain(), storage)

		event := sendPacketEvent(7)
		event.Kind = v2.KindWriteAck

		require.NoError(t, w.HandleEvent(ctx, event))
		assert.Empty(t, storage.written)
	})
}

func TestWatcherStart(t *testing.T) {
	t.Run("subscribesToTheConfiguredClients", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			c := newChain()
			w := newTestWatcher(c, newPacketStore(nil))

			require.NoError(t, w.Start())
			synctest.Wait()

			assert.Equal(t, []string{sourceClientID}, c.clientIDs)
			require.NoError(t, w.Stop())
		})
	})

	t.Run("storeErrorDoesNotKillTheLoop", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			c := newChain()
			storage := newPacketStore(errors.New("store unavailable"))
			w := newTestWatcher(c, storage)

			require.NoError(t, w.Start())
			synctest.Wait()

			c.out <- sendPacketEvent(1)
			c.out <- sendPacketEvent(2)
			synctest.Wait()

			assert.Equal(t, []uint64{1, 2}, storage.sequences())
			require.NoError(t, w.Stop())
		})
	})

	t.Run("stopBlocksUntilTheLoopExits", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			c := newChain()
			w := newTestWatcher(c, newPacketStore(nil))

			require.NoError(t, w.Start())
			synctest.Wait()

			require.NoError(t, w.Stop())

			select {
			case <-w.stopped:
			default:
				t.Fatal("Stop returned before the loop exited")
			}

			// canceling the subscription context is what releases the
			// subscription's goroutine; unsubscribing alone leaves it running
			assert.True(t, c.unsubscribed)
			require.Error(t, c.ctx.Err())
		})
	})

	t.Run("subscribeErrorFailsStart", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			c := newChain()
			c.failNext(errors.New("dial failed"))
			w := newTestWatcher(c, newPacketStore(nil))

			require.ErrorContains(t, w.Start(), "subscribing to send packets")

			// a watcher that never started has nothing to stop, and Stop must
			// not block waiting for a loop that was never running
			require.NoError(t, w.Stop())
		})
	})

	t.Run("stopBeforeStartIsANoop", func(t *testing.T) {
		require.NoError(t, newTestWatcher(newChain(), newPacketStore(nil)).Stop())
	})
}
