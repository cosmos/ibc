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

// Chain represents chain client
type Chain interface {
	GetBlockHeader(ctx context.Context, height uint64) (v2.BlockHeader, error)

	LatestPacketSequence(ctx context.Context, sourceClientID string, height uint64) (uint64, error)
	PacketCommitments(ctx context.Context, sourceClientID string, sequences []uint64, height uint64) ([]uint64, error)
	FindSendPackets(ctx context.Context, sourceClientID string, sequences []uint64) ([]v2.PacketEvent, error)

	SubscribeSendPackets(ctx context.Context, clientIDs []string, out chan<- v2.PacketEvent) (v2.Subscription, error)
}

// PacketStore the persistence discovered packets are written to.
type PacketStore interface {
	UpsertPacket(ctx context.Context, input store.UpsertPacket) error
}

// Config the Watcher configuration.
type Config struct {
	MinBackoff time.Duration
	MaxBackoff time.Duration

	// CleanOnStart runs a pass as soon as the first subscription is live.
	CleanOnStart bool
	// ClearInterval how often a pass runs after that.
	ClearInterval time.Duration
	// AbandonUnrecoverablePackets drops packets whose send log the endpoint will
	// not serve out of the probe set, keeping the record of them.
	AbandonUnrecoverablePackets bool
}

// Watcher records a packet row for every SendPacket event one chain emits on
// the clients it watches, resubscribing with backoff whenever the subscription
// ends. The subscription starts where the chain is and never looks backwards;
// recovering anything it missed is the clearing pass's job.
type Watcher struct {
	chainID   string
	clientIDs []string
	routes    map[string]config.ClientEnd

	chain   Chain
	storage PacketStore

	clearer *Clearer

	cfg Config

	logger *slog.Logger

	cancel  context.CancelFunc
	stopped chan struct{}
}

// stream is one open subscription and the events it feeds. The zero value is
// the gap between a dropped subscription and its replacement.
type stream struct {
	sub    v2.Subscription
	events chan v2.PacketEvent
	cancel context.CancelFunc
}

// New builds the watcher for one chain. The subscriber and the querier are the
// two halves of discovery and are kept apart on purpose: the clearing pass has
// to run when the subscription cannot.
func New(
	chainID string,
	connections []config.ConnectionConfig,
	chain Chain,
	storage ClearStore,
	cfg Config,
	logger *slog.Logger,
) *Watcher {
	if cfg.ClearInterval <= 0 {
		cfg.ClearInterval = config.DefaultClearInterval
	}

	clientIDs := make([]string, 0, len(connections))

	for _, conn := range connections {
		if source, _, ok := conn.SourceEnd(chainID); ok {
			clientIDs = append(clientIDs, source.ClientID)
		}
	}

	clearer := NewClearer(chainID, connections, chain, storage, cfg, logger)

	return &Watcher{
		chainID:   chainID,
		clientIDs: clientIDs,
		routes:    routesOf(chainID, connections),
		chain:     chain,
		storage:   storage,
		clearer:   clearer,
		cfg:       cfg,
		logger:    logger.With("module", "watcher", "chainID", chainID),
	}
}

// Start subscribes and begins the event loop in its own goroutine, failing if
// the subscription cannot be opened.
func (w *Watcher) Start() error {
	ctx, cancel := context.WithCancel(context.Background())

	eventsChan := make(chan v2.PacketEvent, eventBuffer)

	eventStream, err := w.subscribe(ctx, eventsChan)
	if err != nil {
		cancel()

		return err
	}

	w.cancel = cancel
	w.stopped = make(chan struct{})

	go w.run(ctx, eventStream)

	return nil
}

// Stop cancels the subscription loop and blocks until it has exited.
func (w *Watcher) Stop() error {
	if w.cancel == nil {
		return nil
	}

	w.cancel()
	<-w.stopped
	w.cancel = nil

	return nil
}

// HandleEvent records the packet a send event carries. Events of another kind
// and reorged-out logs write nothing.
func (w *Watcher) HandleEvent(ctx context.Context, event v2.PacketEvent) error {
	if event.Kind != v2.KindSendPacket {
		return nil
	}

	if event.Removed {
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

	metrics.sendPacket(ctx, w.chainID)

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

func (w *Watcher) subscribe(ctx context.Context, events chan v2.PacketEvent) (stream, error) {
	// note that for evm implementation cancel() is unused because websocket loop
	// doesn't rely on provided context (see geth's code)
	ctx, cancel := context.WithCancel(ctx)
	sub, err := w.chain.SubscribeSendPackets(ctx, w.clientIDs, events)
	if err != nil {
		cancel()

		return stream{}, errors.Wrap(err, "subscribing to send packets")
	}

	return stream{sub, events, cancel}, nil
}

// todo: "Core Retry Behavior Untested (max backoff, buffered events survive reconnect)"
func (w *Watcher) run(ctx context.Context, eventStream stream) {
	defer func() {
		if err := recover(); err != nil {
			w.logger.Error("Panic recovery in running watcher", "panic", err)
		}
	}()

	defer func() {
		eventStream.close(true)
		close(w.stopped)
	}()

	var (
		// a signal non-nil chan if a pass is in-flight or nil if no pass is in-flight
		// "(we should?) wait for the iteration to complete"
		clearDone chan struct{}
		// shouldClearAgain holds a pass asked for while one was running, so a flapping
		// endpoint coalesces into one follow-up rather than one per reconnect
		shouldClearAgain bool
	)

	// a pass reads the whole sequence space of every watched client, so it runs
	// beside the loop rather than inside it, where it would stall event handling
	triggerClear := func() {
		// no-nop if a pass is already in-flight
		if clearDone != nil {
			return
		}

		done := make(chan struct{})
		clearDone = done
		shouldClearAgain = false

		go func() {
			w.clear(ctx)
			close(done)
		}()
	}

	// Start opened the first subscription, so clearing on start belongs here;
	// every subscribe the loop makes follows a gap, which clearing covers
	if w.cfg.CleanOnStart {
		triggerClear()
	}

	var (
		// nil by default, non-nil when it's time to reconnect
		resubscribeChan <-chan time.Time

		backoff               = w.cfg.MinBackoff
		triggerResubscription = func(msg string, err error) {
			w.logger.Warn(msg, "err", err, "backoff", backoff)

			resubscribeChan = time.After(backoff)
			backoff = min(backoff*2, w.cfg.MaxBackoff)
		}

		// eventsChan *outlives* each subscription, so a reconnect keeps
		// whatever the dropped one had already buffered
		eventsChan = eventStream.events
	)

	clearTick := time.NewTicker(w.cfg.ClearInterval)
	defer clearTick.Stop()

	// combines live events with periodically triggered clearing
	// handles errors, re-subscriptions, and context cancellation
	for {
		select {
		case event := <-eventsChan:
			// basic case A: events come from live subscription
			if err := w.HandleEvent(ctx, event); err != nil {
				w.logger.Error("Recording send packet", "err", err)
			}
		case <-clearTick.C:
			// basic case B: retroactive clearing
			triggerClear()
		case <-ctx.Done():
			// wait for the final iteration to complete
			if clearDone != nil {
				<-clearDone
			}

			return
		case <-clearDone:
			// iteration completed
			clearDone = nil

			if shouldClearAgain {
				triggerClear()
			}
		case err := <-eventStream.errs():
			// close stream and trigger async resubscription (for the next select{} loop)
			eventStream.close(false)
			eventStream = stream{}

			triggerResubscription("Send packet subscription ended, reconnecting", err)
		case <-resubscribeChan:
			newStream, err := w.subscribe(ctx, eventsChan)
			if err != nil {
				triggerResubscription("Subscribing to send packets failed, retrying", err)
				continue
			}

			eventStream = newStream
			resubscribeChan = nil

			// todo: "backoff resets too early — Subscribe success resets delay before the replacement is healthy"
			backoff = w.cfg.MinBackoff

			// it will either instantly triggerClear() or act as marker to triggerClear() as soon
			// as the current pass completes (`case <-clearDone`)
			shouldClearAgain = true
			triggerClear()

			w.logger.Info("Resubscribed to send packets", "clientIDs", w.clientIDs)
		}
	}
}

// clear runs one pass over every watched client. A client that fails is logged
// and left for the next pass: the backstop going quiet must not take the
// subscription down with it.
func (w *Watcher) clear(ctx context.Context) {
	for _, clientID := range w.clientIDs {
		if ctx.Err() != nil {
			return
		}

		started := time.Now()

		result, err := w.clearer.Clear(ctx, clientID)
		if err != nil {
			w.logger.Error("Clearing outstanding packets", "clientID", clientID, "err", err)
			continue
		}

		w.logger.Info(
			"Cleared outstanding packets",
			"clientID", clientID,
			"probed", result.Probed,
			"outstanding", result.Outstanding,
			"alreadyHeld", result.AlreadyHeld,
			"recovered", result.Recovered,
			"unresolved", result.Unresolved,
			"abandoned", result.Abandoned,
			"took", time.Since(started),
		)
	}
}

// close releases all of a subscription's resources
func (s stream) close(closeChan bool) {
	if s.sub == nil {
		return
	}

	s.sub.Unsubscribe()
	s.cancel()

	if closeChan {
		close(s.events)
	}
}

// errs is nil while nothing is subscribed, so the loop simply waits out a gap.
func (s stream) errs() <-chan error {
	if s.sub == nil {
		return nil
	}

	return s.sub.Err()
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

func packetRow(chainID, destChainID string, event v2.PacketEvent) store.UpsertPacket {
	//nolint:gosec // timeout timestamps fit in int64
	timeout := time.Unix(int64(event.Packet.TimeoutTimestamp), 0).UTC()

	return store.UpsertPacket{
		Status:                    store.RelayStatusPending,
		SourceChainID:             chainID,
		DestinationChainID:        destChainID,
		SourceTxHash:              event.TxHash,
		SourceTxTime:              event.BlockTime,
		PacketSequenceNumber:      event.Packet.Sequence,
		PacketSourceClientID:      event.Packet.SourceClient,
		PacketDestinationClientID: event.Packet.DestinationClient,
		PacketTimeoutTimestamp:    timeout,
	}
}
