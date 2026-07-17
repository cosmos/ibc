package pipeline

import (
	"context"
	"log/slog"
	"time"

	"github.com/deliveryhero/pipeline/v2"

	"github.com/cosmos/ibc/link/internal/relay/transfer"
)

// ConditionallyBatchProcess batches the transfers the processor applies to
// (per ShouldProcess) and forwards everything else downstream immediately.
// Batches are released at maxSize or after maxDuration, collected by a single
// collector, and executed by up to concurrency processors.
func ConditionallyBatchProcess(
	ctx context.Context,
	logger *slog.Logger,
	concurrency int,
	maxSize int,
	maxDuration time.Duration,
	in <-chan *transfer.Transfer,
	processor BatchProcessorMW,
) <-chan *transfer.Transfer {
	out := make(chan *transfer.Transfer)
	toBatch := make(chan *transfer.Transfer)

	go func() {
		for i := range in {
			if i == nil {
				continue
			}

			if i.Error() != "" {
				// errored transfers skip batching and flow through immediately
				out <- i

				continue
			}

			if !processor.ShouldProcess(i) {
				// transfers this processor does not apply to flow through
				out <- i

				continue
			}

			i.GetLogger().Debug("Pushing tr into batch", "status", processor.Status())
			toBatch <- i
		}

		// closing the collector input eventually closes the batch results,
		// which closes out
		close(toBatch)
	}()

	// collect transfers until the batch reaches max size or has waited for
	// max duration
	batches := pipeline.Collect(ctx, maxSize, maxDuration, toBatch)

	// batch processing can be slow (an on-chain submission); if collected
	// batches were handed to the processors synchronously, a slow batch would
	// stop the collector from reading its input and stall the entire
	// pipeline. buffering the collected batches keeps upstream stages flowing.
	const bufferSize = 1000

	bufferedBatches := make(chan []*transfer.Transfer, bufferSize)
	go func() {
		for batch := range batches {
			bufferedBatches <- batch

			if len(bufferedBatches) >= 3 {
				logger.Warn(
					"Batches accumulating in buffer",
					"status", processor.Status(),
					"bufferedBatches", len(bufferedBatches),
					"maxBufferSize", bufferSize,
				)
			}
		}

		close(bufferedBatches)
	}()

	batchResults := pipeline.ProcessConcurrently(ctx, concurrency, processor, bufferedBatches)

	go func() {
		// pulling results happens in its own goroutine: if more than a batch
		// worth of transfers are waiting on toBatch, the processor blocks on
		// pushing results unless someone is draining them
		for batchResult := range batchResults {
			for _, output := range batchResult {
				out <- output
			}
		}

		close(out)
	}()

	return out
}
