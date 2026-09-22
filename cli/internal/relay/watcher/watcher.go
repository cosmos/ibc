// SPDX-License-Identifier: Apache-2.0
package watcher

import (
	"context"
	"log/slog"
	"sync"
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

// Watcher watches live chain events + runs a periodic clearing pass to fetch missed packets.
// All discovered packets are written to the storage.
type Watcher struct {
	chainID   string
	clientIDs []string
	routes    map[string]config.ClientEnd
	cfg       Config

	chain   Chain
	storage PacketStore

	clearer  *Clearer
	clearNow chan struct{}

	logger *slog.Logger

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// stream is one open subscription and the events it feeds. The zero value is
// the gap between a dropped subscription and its replacement.
type stream struct {
	sub    v2.Subscription
	events chan v2.PacketEvent
	cancel context.CancelFunc
}

// New Watcher constructor.
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
		cfg:       cfg,

		chain:   chain,
		storage: storage,

		// buf of 1 allows instant clearance after wss reconnect
		clearNow: make(chan struct{}, 1),
		clearer:  clearer,

		logger: logger.With("module", "watcher", "chainID", chainID),
	}
}

func (w *Watcher) Start() error {
	ctx, cancel := context.WithCancel(context.Background())

	eventsChan := make(chan v2.PacketEvent, eventBuffer)

	eventStream, err := w.subscribe(ctx, eventsChan)
	if err != nil {
		cancel()

		return err
	}

	w.cancel = cancel
	w.wg.Add(2)

	// note that these routines have no panic recovery on purpose.
	// panic in clearing/subscription routine explicitly crashes the program.
	go func() {
		defer w.wg.Done()
		w.runClearer(ctx)
	}()

	go func() {
		defer w.wg.Done()
		w.runSubscription(ctx, eventStream)
	}()

	return nil
}

func (w *Watcher) Stop() error {
	if w.cancel == nil {
		return nil
	}

	w.cancel()
	w.wg.Wait()
	w.cancel = nil

	return nil
}

// HandleEvent records the packet a send event carries.
// Events of another kind and reorged-out logs write nothing.
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

// connects to live events and recovers broken subscriptions.
func (w *Watcher) runSubscription(ctx context.Context, eventStream stream) {
	defer func() {
		eventStream.close(true)
	}()

	// nil by default, non-nil when it's time to reconnect
	var reconnectChan <-chan time.Time

	backoff := w.cfg.MinBackoff
	triggerReconnect := func(msg string, err error) {
		w.logger.Warn(msg, "backoff", backoff.String(), "err", err)
		reconnectChan = time.After(backoff)
		backoff = min(backoff*2, w.cfg.MaxBackoff)
	}

	// eventsChan *outlives* each subscription, so a reconnect
	// keeps whatever the dropped one had already buffered.
	eventsChan := eventStream.events

	for {
		select {
		case event := <-eventsChan:
			if err := w.HandleEvent(ctx, event); err != nil {
				w.logger.Error("Recording send packet", "err", err)
			}
		case <-ctx.Done():
			return
		case err := <-eventStream.errs():
			eventStream.close(false)
			eventStream = stream{}
			triggerReconnect("Send packet subscription ended, reconnecting", err)
		case <-reconnectChan:
			newStream, err := w.subscribe(ctx, eventsChan)
			if err != nil {
				triggerReconnect("Subscribing to send packets failed, retrying", err)
				continue
			}

			eventStream = newStream
			reconnectChan = nil

			// future: consider gradual backoff decrease
			backoff = w.cfg.MinBackoff

			w.logger.Info("Resubscribed to send packets", "clientIDs", w.clientIDs)

			// trigger clearer
			w.clearRequest()
		}
	}
}

func (w *Watcher) runClearer(ctx context.Context) {
	// optional first iteration
	if w.cfg.CleanOnStart {
		w.clear(ctx)
	}

	ticker := time.NewTicker(w.cfg.ClearInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			w.clear(ctx)
		case <-w.clearNow:
			w.clear(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (w *Watcher) clearRequest() {
	select {
	case w.clearNow <- struct{}{}:
	default:
		// already running
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
			"elapsed", time.Since(started).String(),
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
