package pipeline

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/cosmos/ibc/link/internal/relayer/transfer"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/store"
)

func TestConditionallyBatchProcess(t *testing.T) {
	ctx := context.Background()

	collect := func(out <-chan *transfer.Transfer, n int) []*transfer.Transfer {
		var got []*transfer.Transfer
		for tr := range out {
			got = append(got, tr)
			if len(got) == n {
				break
			}
		}

		return got
	}

	t.Run("batchesBySize", func(t *testing.T) {
		storage := &statusRecorder{}
		internal := &fakeBatchProcessor{shouldProcess: func(*transfer.Transfer) bool { return true }}
		in := make(chan *transfer.Transfer)

		out := ConditionallyBatchProcess(ctx, slog.Default(), 1, 2, time.Minute, in, NewBatchProcessorMW(storage, internal))

		go func() {
			in <- testTransfer(t)
			in <- testTransfer(t)
			close(in)
		}()

		got := collect(out, 2)

		assert.Len(t, got, 2)
		require.Len(t, internal.processed, 1)
		assert.Len(t, internal.processed[0], 2)
	})

	t.Run("batchesByTimeout", func(t *testing.T) {
		storage := &statusRecorder{}
		internal := &fakeBatchProcessor{shouldProcess: func(*transfer.Transfer) bool { return true }}
		in := make(chan *transfer.Transfer)

		out := ConditionallyBatchProcess(ctx, slog.Default(), 1, 50, 50*time.Millisecond, in, NewBatchProcessorMW(storage, internal))

		go func() {
			in <- testTransfer(t)
		}()

		got := collect(out, 1)

		assert.Len(t, got, 1)
		require.Len(t, internal.processed, 1)
		assert.Len(t, internal.processed[0], 1)

		close(in)
	})

	t.Run("bypassesNonApplicableAndErrored", func(t *testing.T) {
		storage := &statusRecorder{}
		internal := &fakeBatchProcessor{shouldProcess: func(*transfer.Transfer) bool { return false }}
		in := make(chan *transfer.Transfer)

		out := ConditionallyBatchProcess(ctx, slog.Default(), 1, 2, time.Hour, in, NewBatchProcessorMW(storage, internal))

		errored := testTransfer(t)
		errored.ProcessingError = errors.New("poisoned")

		go func() {
			in <- testTransfer(t)
			in <- errored
			close(in)
		}()

		// both arrive without any batch having been released (timeout is 1h)
		got := collect(out, 2)

		assert.Len(t, got, 2)
		assert.Empty(t, internal.processed)
	})

	t.Run("collectsWhileProcessing", func(t *testing.T) {
		storage := &statusRecorder{}

		release := make(chan struct{})
		slow := &blockingBatchProcessor{release: release}
		in := make(chan *transfer.Transfer)

		out := ConditionallyBatchProcess(ctx, slog.Default(), 1, 1, time.Hour, in, NewBatchProcessorMW(storage, slow))

		// the first batch blocks in the processor; the collector must keep
		// accepting input regardless
		done := make(chan struct{})
		go func() {
			for range 5 {
				in <- testTransfer(t)
			}
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("collector stalled while a batch was processing")
		}

		close(release)
		collect(out, 5)
		close(in)
	})
}

// blockingBatchProcessor blocks every Process call until released.
type blockingBatchProcessor struct {
	release <-chan struct{}
}

func (p *blockingBatchProcessor) ShouldProcess(*transfer.Transfer) bool { return true }

func (p *blockingBatchProcessor) Status() store.RelayStatus {
	return store.RelayStatusDeliverRecvPacket
}

func (p *blockingBatchProcessor) Process(_ context.Context, batch []*transfer.Transfer) ([]*transfer.Transfer, error) {
	<-p.release

	return batch, nil
}

func (p *blockingBatchProcessor) Cancel([]*transfer.Transfer, error) {}
