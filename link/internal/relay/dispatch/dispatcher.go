package dispatch

import (
	"context"
	"log/slog"
	"time"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/relay/processors"
	"github.com/cosmos/ibc/link/internal/store"
)

// DefaultPollInterval how often the dispatcher polls for unfinished packets.
const DefaultPollInterval = 5 * time.Second

// ErrTransferAlreadyInPipeline the transfer is already being relayed.
var ErrTransferAlreadyInPipeline = errors.New("transfer already in pipeline")

// DispatcherStorage the persistence used by the dispatcher.
type DispatcherStorage interface {
	ListUnfinishedPackets(ctx context.Context) ([]store.Packet, error)
	UpdatePacketStatus(ctx context.Context, key store.PacketKey, status store.RelayStatus) error
}

// RelayDispatcher polls the store for unfinished packets and routes them to
// their pipelines. The relay API couples to the dispatcher through the store:
// it inserts packets, the dispatcher picks them up on the next poll.
type RelayDispatcher struct {
	storage      DispatcherStorage
	pipelines    Pipelines
	pollInterval time.Duration
	logger       *slog.Logger

	cancel  context.CancelFunc
	stopped chan struct{}
}

func NewRelayDispatcher(
	storage DispatcherStorage,
	pipelines Pipelines,
	pollInterval time.Duration,
	logger *slog.Logger,
) *RelayDispatcher {
	return &RelayDispatcher{
		storage:      storage,
		pipelines:    pipelines,
		pollInterval: pollInterval,
		logger:       logger.With("module", "dispatcher"),
	}
}

// Start begins the dispatch loop in its own goroutine.
// Polls for unfinished packets until Stop is called.
func (d *RelayDispatcher) Start() error {
	ctx, cancel := context.WithCancel(context.Background())
	d.cancel = cancel
	d.stopped = make(chan struct{})

	go func() {
		defer close(d.stopped)

		// fire immediately, then at the poll interval
		ticker := time.NewTicker(time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				d.pipelines.Close()

				return
			case <-ticker.C:
				ticker.Reset(d.pollInterval)

				if err := d.SubmitWaitingUnfinishedPackets(ctx); err != nil {
					d.logger.Error("Submitting unfinished packets", "error", err)
				}
			}
		}
	}()

	return nil
}

// Stop cancels the dispatch loop and blocks until it has exited and every
// pipeline it created is closed.
func (d *RelayDispatcher) Stop() error {
	if d.cancel == nil {
		return nil
	}

	d.cancel()
	<-d.stopped

	return nil
}

// SubmitWaitingUnfinishedPackets pushes every unfinished packet into its
// pipeline. Packets that fail to submit for anything other than already being
// in flight are marked failed: submission only fails on configuration errors
// that will not resolve by retrying.
func (d *RelayDispatcher) SubmitWaitingUnfinishedPackets(ctx context.Context) error {
	packets, err := d.storage.ListUnfinishedPackets(ctx)
	if err != nil {
		return errors.Wrap(err, "listing unfinished packets")
	}

	for _, packet := range packets {
		tr := processors.NewTransfer(packet, d.logger)

		err := d.SubmitTransfer(ctx, tr)
		switch {
		case errors.Is(err, ErrTransferAlreadyInPipeline):
			continue
		case err != nil:
			tr.GetLogger().Error("Submitting transfer failed, marking packet failed", "error", err)

			if errUpdate := d.storage.UpdatePacketStatus(ctx, tr.Key(), store.RelayStatusFailed); errUpdate != nil {
				tr.GetLogger().Error("Marking packet failed", "error", errUpdate)
			}
		}
	}

	return nil
}

// SubmitTransfer routes the transfer to its pipeline and pushes it.
func (d *RelayDispatcher) SubmitTransfer(ctx context.Context, tr *processors.Transfer) error {
	pl, err := d.pipelines.Pipeline(ctx, tr)
	if err != nil {
		return errors.Wrap(err, "getting pipeline for transfer")
	}

	if !pl.Push(ctx, tr) {
		return ErrTransferAlreadyInPipeline
	}

	return nil
}
