// SPDX-License-Identifier: Apache-2.0
package watcher

import (
	"context"
	"log/slog"
	"time"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/cli/internal/config"
	"github.com/cosmos/ibc/cli/internal/store"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

// Backoff bounds for reconnecting a dropped subscription.
const (
	DefaultMinBackoff = time.Second
	DefaultMaxBackoff = time.Minute
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
// the clients it watches, resubscribing with backoff whenever the subscription
// ends. The subscription starts where the chain is and never looks backwards,
// so a packet sent while nothing was listening is not discovered here.
type Watcher struct {
	chainID    string
	clientIDs  []string
	routes     map[string]config.ClientEnd
	subscriber Subscriber
	storage    PacketStore
	minBackoff time.Duration
	maxBackoff time.Duration
	logger     *slog.Logger

	cancel  context.CancelFunc
	stopped chan struct{}
}

// New builds the watcher for one chain.
func New(
	chainID string,
	connections []config.ConnectionConfig,
	subscriber Subscriber,
	storage PacketStore,
	minBackoff, maxBackoff time.Duration,
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
		routes:     routesOf(chainID, connections),
		subscriber: subscriber,
		storage:    storage,
		minBackoff: minBackoff,
		maxBackoff: maxBackoff,
		logger:     logger.With("module", "watcher", "chainID", chainID),
	}
}

// routesOf maps each watched client to the end its packets are relayed to.
func routesOf(chainID string, connections []config.ConnectionConfig) map[string]config.ClientEnd {
	routes := make(map[string]config.ClientEnd, len(connections))

	for _, conn := range connections {
		if source, destination, ok := conn.SourceEnd(chainID); ok {
			routes[source.ClientID] = destination
		}
	}

	return routes
}

// Start subscribes and begins the event loop in its own goroutine, failing if
// the subscription cannot be opened.
func (w *Watcher) Start() error {
	ctx, cancel := context.WithCancel(context.Background())

	stream, err := w.subscribe(ctx, make(chan v2.PacketEvent, eventBuffer))
	if err != nil {
		cancel()

		return err
	}

	w.cancel = cancel
	w.stopped = make(chan struct{})

	go w.run(ctx, stream)

	return nil
}

// stream is one open subscription and the events it feeds. The zero value is
// the gap between a dropped subscription and its replacement.
type stream struct {
	sub    v2.Subscription
	events chan v2.PacketEvent
	cancel context.CancelFunc
}

// close releases all of a subscription's resources
func (s stream) close() {
	if s.sub == nil {
		return
	}

	s.sub.Unsubscribe()
	s.cancel()
}

// errs is nil while nothing is subscribed, so the loop simply waits out a gap.
func (s stream) errs() <-chan error {
	if s.sub == nil {
		return nil
	}

	return s.sub.Err()
}

func (w *Watcher) subscribe(ctx context.Context, events chan v2.PacketEvent) (stream, error) {
	subCtx, cancel := context.WithCancel(ctx)
	sub, err := w.subscriber.SubscribeSendPackets(subCtx, w.clientIDs, events)
	if err != nil {
		cancel()

		return stream{}, errors.Wrap(err, "subscribing to send packets")
	}

	return stream{sub: sub, events: events, cancel: cancel}, nil
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

func (w *Watcher) run(ctx context.Context, open stream) {
	defer close(w.stopped)
	defer func() { open.close() }()

	defer func() {
		if err := recover(); err != nil {
			w.logger.Error("Panic recovery in running watcher", "panic", err)
		}
	}()

	// the events channel outlives each subscription, so a reconnect keeps
	// whatever the dropped one had already buffered
	events := open.events
	backoff := w.minBackoff

	// nil while a subscription is live, so only a gap paces itself
	var resubscribe <-chan time.Time

	retryIn := func(msg string, err error) {
		w.logger.Warn(msg, "err", err, "backoff", backoff)

		resubscribe = time.After(backoff)
		backoff = min(backoff*2, w.maxBackoff)
	}

	for {
		select {
		case <-ctx.Done():
			return

		case event := <-events:
			if err := w.HandleEvent(ctx, event); err != nil {
				w.logger.Error("Recording send packet", "err", err)
			}

		case err := <-open.errs():
			open.close()
			open = stream{}

			retryIn("Send packet subscription ended, reconnecting", err)

		case <-resubscribe:
			opened, err := w.subscribe(ctx, events)
			if err != nil {
				retryIn("Subscribing to send packets failed, retrying", err)

				continue
			}

			open, resubscribe, backoff = opened, nil, w.minBackoff

			w.logger.Info("Subscribed to send packets", "clientIDs", w.clientIDs)
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

	// the subscription filters on the source client alone, so the counterparty
	// the packet actually names is checked here, as the explicit relay path does
	route, configured := w.routes[event.Packet.SourceClient]

	switch {
	case !configured:
		w.logger.Warn(
			"Skipping packet from unconfigured client",
			"clientID", event.Packet.SourceClient,
			"sequence", event.Packet.Sequence,
		)
		return nil
	case event.Packet.DestinationClient != route.ClientID:
		w.logger.Debug(
			"Skipping packet with unconfigured destination client",
			"clientID", event.Packet.SourceClient,
			"destinationClientID", event.Packet.DestinationClient,
			"sequence", event.Packet.Sequence,
		)
		return nil
	}

	w.logger.Debug(
		"Send packet observed",
		"clientID", event.Packet.SourceClient,
		"sequence", event.Packet.Sequence,
		"txHash", event.TxHash,
	)

	row := packetRow(w.chainID, route.ChainID, event)
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
