package pipeline

import (
	"context"
	"log/slog"
	"time"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/store"
)

// DefaultPollInterval how often the dispatcher polls for unfinished packets.
const DefaultPollInterval = 5 * time.Second

// ErrTransferAlreadyInPipeline the transfer is already being relayed.
var ErrTransferAlreadyInPipeline = errors.New("transfer already in pipeline")

// DispatcherStorage the persistence used by the dispatcher.
type DispatcherStorage interface {
	ListUnfinishedPackets(ctx context.Context) ([]store.Packet, error)
	StatusStorage
}

// RelayDispatcher polls the store for unfinished packets and routes them to
// their pipelines. The relay API couples to the dispatcher through the store:
// it inserts packets, the dispatcher picks them up on the next poll.
type RelayDispatcher struct {
	storage      DispatcherStorage
	manager      RouteManager
	pollInterval time.Duration
	logger       *slog.Logger
}

func NewRelayDispatcher(
	storage DispatcherStorage,
	manager RouteManager,
	pollInterval time.Duration,
	logger *slog.Logger,
) *RelayDispatcher {
	return &RelayDispatcher{
		storage:      storage,
		manager:      manager,
		pollInterval: pollInterval,
		logger:       logger.With("module", "dispatcher"),
	}
}

// Run submits unfinished packets on every poll until the context is canceled.
func (d *RelayDispatcher) Run(ctx context.Context) error {
	// fire immediately, then at the poll interval
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			d.manager.Close()

			return ctx.Err()
		case <-ticker.C:
			ticker.Reset(d.pollInterval)

			if err := d.SubmitWaitingUnfinishedPackets(ctx); err != nil {
				d.logger.Error("Submitting unfinished packets", "error", err)
			}
		}
	}
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
		transfer := NewTransfer(packet, d.logger)

		err := d.SubmitTransfer(ctx, transfer)
		switch {
		case errors.Is(err, ErrTransferAlreadyInPipeline):
			continue
		case err != nil:
			transfer.GetLogger().Error("Submitting transfer failed, marking packet failed", "error", err)

			if errUpdate := d.storage.UpdatePacketStatus(ctx, transfer.Key(), store.RelayStatusFailed); errUpdate != nil {
				transfer.GetLogger().Error("Marking packet failed", "error", errUpdate)
			}
		}
	}

	return nil
}

// SubmitTransfer routes the transfer to its pipeline and pushes it.
func (d *RelayDispatcher) SubmitTransfer(ctx context.Context, transfer *Transfer) error {
	pipeline, err := d.manager.Pipeline(ctx, transfer)
	if err != nil {
		return errors.Wrap(err, "getting pipeline for transfer")
	}

	if !pipeline.Push(ctx, transfer) {
		return ErrTransferAlreadyInPipeline
	}

	return nil
}
