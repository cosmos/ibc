// SPDX-License-Identifier: Apache-2.0
package watcher

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/pkg/errors"

	"github.com/lfdt-ibc/ibc/cli/internal/config"
	"github.com/lfdt-ibc/ibc/cli/internal/store"
	v2 "github.com/lfdt-ibc/ibc/cli/internal/types/v2"
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
	routes    routes
	cfg       Config

	chain   Chain
	storage PacketStore

	clearer  *Clearer
	clearNow chan struct{}

	logger *slog.Logger

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// stream is one open subscription. The zero value is the gap before the
// first subscription and between a dropped one and its replacement.
type stream struct {
	sub    v2.Subscription
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

// Start launches the clearing and subscription loops. A subscription that
// cannot be opened is retried with backoff; the clearer covers the gap.
func (w *Watcher) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel

	// note these routines have no panic recovery on purpose.
	// panic in clearing/subscription routine explicitly crashes the program.
	w.wg.Go(func() {
		w.runClearer(ctx)
	})

	w.wg.Go(func() {
		w.runSubscription(ctx)
	})
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

	route, ok := w.routes.destination(event)
	if !ok {
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

	metrics.subscription(ctx, w.chainID, true)

	return stream{sub, cancel}, nil
}

// connects to live events and recovers broken subscriptions. The first
// subscribe is attempted at once; it and every later one retry with backoff.
func (w *Watcher) runSubscription(ctx context.Context) {
	metrics.subscription(ctx, w.chainID, false)

	backoff := w.cfg.MinBackoff

	// nil while subscribed, non-nil when it's time to (re)connect
	var reconnectChan <-chan time.Time

	triggerReconnect := func(msg string, err error) {
		w.logger.Warn(msg, "backoff", backoff.String(), "err", err)
		reconnectChan = time.After(backoff)
		backoff = min(backoff*2, w.cfg.MaxBackoff)
	}

	// eventsChan *outlives* each subscription, so a reconnect
	// keeps whatever the dropped one had already buffered.
	eventsChan := make(chan v2.PacketEvent, eventBuffer)

	eventStream, err := w.subscribe(ctx, eventsChan)
	if err != nil {
		triggerReconnect("Subscribing to send packets failed, retrying", err)
	}

	defer func() {
		metrics.subscription(ctx, w.chainID, false)
		eventStream.close()
	}()

	for {
		select {
		case event := <-eventsChan:
			if err := w.HandleEvent(ctx, event); err != nil {
				w.logger.Error("Recording send packet", "err", err)
			}
		case <-ctx.Done():
			return
		case err := <-eventStream.errs():
			metrics.subscription(ctx, w.chainID, false)
			eventStream.close()
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

			w.logger.Info("Subscribed to send packets", "clientIDs", w.clientIDs)

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
			"commitmentsQueried", result.CommitmentsQueried,
			"commitmentsLive", result.CommitmentsLive,
			"packetsAlreadyStored", result.PacketsAlreadyStored,
			"packetsRecovered", result.PacketsRecovered,
			"seqsUnresolved", result.SeqsUnresolved,
			"seqsAbandoned", result.SeqsAbandoned,
			"elapsed", time.Since(started).String(),
		)
	}
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

// routes maps each watched client to the end its packets are relayed to.
type routes map[string]config.ClientEnd

func routesOf(chainID string, connections []config.ConnectionConfig) routes {
	r := make(routes, len(connections))

	for _, conn := range connections {
		if source, destination, ok := conn.SourceEnd(chainID); ok {
			r[source.ClientID] = destination
		}
	}

	return r
}

// destination returns the end a send packet is relayed to. Chains filter sends
// on the source client alone, so a packet naming a counterparty other than the
// configured one is not ours to relay, the same check the explicit relay path
// makes.
func (r routes) destination(event v2.PacketEvent) (config.ClientEnd, bool) {
	route, ok := r[event.Packet.SourceClient]
	if !ok || event.Packet.DestinationClient != route.ClientID {
		return config.ClientEnd{}, false
	}

	return route, true
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
