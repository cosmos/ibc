// SPDX-License-Identifier: Apache-2.0
package watcher

import (
	"context"
	"log/slog"
	"time"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/store"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

// eventBuffer matches the log buffer the chain client subscribes with, so a
// slow store write does not immediately back up the websocket.
const eventBuffer = 128

// Subscriber the chain-side event stream.
type Subscriber interface {
	SubscribeSendPackets(ctx context.Context, clientIDs []string, out chan<- v2.PacketEvent) (v2.Subscription, error)
}

// PacketStore the persistence discovered packets are written to.
type PacketStore interface {
	UpsertPacket(ctx context.Context, input store.UpsertPacket) error
}

// Watcher records a packet row for every SendPacket event one chain emits on
// the clients it watches. The subscription starts where the chain is and never
// looks backwards, so a packet sent while nothing was listening is not
// discovered here.
type Watcher struct {
	chainID    string
	clientIDs  []string
	destChains map[string]string
	subscriber Subscriber
	storage    PacketStore

	cancel  context.CancelFunc
	stopped chan struct{}

	logger *slog.Logger
}

// New builds the watcher for one chain.
func New(
	chainID string,
	connections []config.ConnectionConfig,
	subscriber Subscriber,
	storage PacketStore,
	logger *slog.Logger,
) *Watcher {
	clientIDs := make([]string, 0, len(connections))

	for _, conn := range connections {
		if source, _, ok := conn.SourceEnd(chainID); ok {
			clientIDs = append(clientIDs, source.ClientID)
		}
	}

	return &Watcher{
		chainID:    chainID,
		clientIDs:  clientIDs,
		destChains: destChainsOf(chainID, connections),
		subscriber: subscriber,
		storage:    storage,
		logger:     logger.With("module", "watcher", "chainID", chainID),
	}
}

// destChainsOf maps each watched client to the chain its packets are relayed to.
func destChainsOf(chainID string, connections []config.ConnectionConfig) map[string]string {
	destChains := make(map[string]string, len(connections))

	for _, conn := range connections {
		if source, destination, ok := conn.SourceEnd(chainID); ok {
			destChains[source.ClientID] = destination.ChainID
		}
	}

	return destChains
}

// Start begins the subscription loop in its own goroutine.
func (w *Watcher) Start() error {
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.stopped = make(chan struct{})

	go w.run(ctx)

	return nil
}

// Stop cancels the subscription loop and blocks until it has exited.
func (w *Watcher) Stop() error {
	if w.cancel == nil {
		return nil
	}

	w.cancel()
	<-w.stopped

	return nil
}

func (w *Watcher) run(ctx context.Context) {
	defer close(w.stopped)

	events := make(chan v2.PacketEvent, eventBuffer)

	// canceling the per-subscription context is what releases the
	// subscription's goroutine; unsubscribing alone leaves it running
	subCtx, subCancel := context.WithCancel(ctx)
	defer subCancel()

	sub, err := w.subscriber.SubscribeSendPackets(subCtx, w.clientIDs, events)
	if err != nil {
		w.logger.Error("Subscribing to send packets failed", "err", err)
		return
	}
	defer sub.Unsubscribe()

	w.logger.Info("Subscribed to send packets", "clientIDs", w.clientIDs)

	for {
		select {
		case <-ctx.Done():
			return

		case event := <-events:
			if err := w.HandleEvent(ctx, event); err != nil {
				w.logger.Error("Recording send packet", "err", err)
			}

		case err := <-sub.Err():
			w.logger.Error("Send packet subscription ended", "err", err)

			return
		}
	}
}

// HandleEvent records the packet a send event carries. Events of another kind
// and reorged-out logs write nothing.
func (w *Watcher) HandleEvent(ctx context.Context, event v2.PacketEvent) error {
	switch {
	case event.Kind != v2.KindSendPacket:
		return nil
	case event.Removed:
		// deleting the row would destroy the record of a relay we may already
		// have submitted, so the row stands and the pipeline keeps retrying it
		// or performs its own reorg check, we do not handle this case
		// specifically in the watcher.
		w.logger.Warn(
			"Send packet reorged out, leaving its row in place",
			"clientID", event.Packet.SourceClient,
			"sequence", event.Packet.Sequence,
			"txHash", event.TxHash,
		)

		return nil
	}

	w.logger.Debug(
		"Send packet observed",
		"clientID", event.Packet.SourceClient,
		"sequence", event.Packet.Sequence,
		"txHash", event.TxHash,
	)

	row := packetRow(w.chainID, w.destChains[event.Packet.SourceClient], event)

	if err := w.storage.UpsertPacket(ctx, row); err != nil {
		return errors.Wrapf(
			err,
			"creating packet %d for client %s",
			event.Packet.Sequence, event.Packet.SourceClient,
		)
	}
	return nil
}

func packetRow(chainID, destChainID string, event v2.PacketEvent) store.UpsertPacket {
	return store.UpsertPacket{
		Status:                    store.RelayStatusPending,
		SourceChainID:             chainID,
		DestinationChainID:        destChainID,
		SourceTxHash:              event.TxHash,
		SourceTxTime:              event.BlockTime,
		PacketSequenceNumber:      event.Packet.Sequence,
		PacketSourceClientID:      event.Packet.SourceClient,
		PacketDestinationClientID: event.Packet.DestinationClient,
		//nolint:gosec // timeout timestamps fit in int64
		PacketTimeoutTimestamp: time.Unix(int64(event.Packet.TimeoutTimestamp), 0).UTC(),
	}
}
