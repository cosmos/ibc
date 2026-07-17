package pipeline

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/cosmos/ibc/link/internal/relay/processors"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/store"
)

// fakeProcessor is a hand-rolled Processor with programmable behavior.
type fakeProcessor struct {
	shouldProcess bool
	processErr    error
	processed     []*processors.Transfer
	cancelled     []*processors.Transfer
}

func (p *fakeProcessor) ShouldProcess(*processors.Transfer) bool { return p.shouldProcess }
func (p *fakeProcessor) Status() store.RelayStatus               { return store.RelayStatusDeliverRecvPacket }

func (p *fakeProcessor) Process(_ context.Context, t *processors.Transfer) (*processors.Transfer, error) {
	if p.processErr != nil {
		return nil, p.processErr
	}

	p.processed = append(p.processed, t)

	return t, nil
}

func (p *fakeProcessor) Cancel(t *processors.Transfer, _ error) { p.cancelled = append(p.cancelled, t) }

// fakeBatchProcessor mirrors fakeProcessor for batches.
type fakeBatchProcessor struct {
	shouldProcess func(*processors.Transfer) bool
	processErr    error
	processed     [][]*processors.Transfer
	cancelled     [][]*processors.Transfer
}

func (p *fakeBatchProcessor) ShouldProcess(t *processors.Transfer) bool { return p.shouldProcess(t) }
func (p *fakeBatchProcessor) Status() store.RelayStatus                 { return store.RelayStatusDeliverRecvPacket }

func (p *fakeBatchProcessor) Process(_ context.Context, batch []*processors.Transfer) ([]*processors.Transfer, error) {
	if p.processErr != nil {
		return nil, p.processErr
	}

	p.processed = append(p.processed, batch)

	return batch, nil
}

func (p *fakeBatchProcessor) Cancel(batch []*processors.Transfer, _ error) {
	p.cancelled = append(p.cancelled, batch)
}

type statusRecorder struct {
	updates []store.RelayStatus
	err     error
}

func (s *statusRecorder) UpdatePacketStatus(_ context.Context, _ store.PacketKey, status store.RelayStatus) error {
	if s.err != nil {
		return s.err
	}

	s.updates = append(s.updates, status)

	return nil
}

func testTransfer(t *testing.T) *processors.Transfer {
	t.Helper()

	return processors.NewTransfer(store.Packet{
		Status:                    store.RelayStatusPending,
		SourceChainID:             "1",
		DestinationChainID:        "8453",
		SourceTxHash:              "0xsend",
		PacketSequenceNumber:      42,
		PacketSourceClientID:      "base-0",
		PacketDestinationClientID: "ethereum-0",
		PacketTimeoutTimestamp:    time.Now().Add(time.Hour),
	}, slog.Default())
}

func TestProcessorMW(t *testing.T) {
	ctx := context.Background()

	t.Run("persistsStatusAndProcesses", func(t *testing.T) {
		storage := &statusRecorder{}
		internal := &fakeProcessor{shouldProcess: true}
		mw := NewProcessorMW(storage, internal)

		out, err := mw.Process(ctx, testTransfer(t))

		require.NoError(t, err)
		assert.Equal(t, store.RelayStatusDeliverRecvPacket, out.Status)
		assert.Equal(t, []store.RelayStatus{store.RelayStatusDeliverRecvPacket}, storage.updates)
		assert.Len(t, internal.processed, 1)
	})

	t.Run("skipsErroredTransfers", func(t *testing.T) {
		storage := &statusRecorder{}
		internal := &fakeProcessor{shouldProcess: true}
		mw := NewProcessorMW(storage, internal)

		tr := testTransfer(t)
		tr.ProcessingError = errors.New("poisoned")

		out, err := mw.Process(ctx, tr)

		require.NoError(t, err)
		assert.Same(t, tr, out)
		assert.Empty(t, storage.updates)
		assert.Empty(t, internal.processed)
	})

	t.Run("skipsFailedTransfers", func(t *testing.T) {
		storage := &statusRecorder{}
		internal := &fakeProcessor{shouldProcess: true}
		mw := NewProcessorMW(storage, internal)

		tr := testTransfer(t)
		tr.Status = store.RelayStatusFailed

		_, err := mw.Process(ctx, tr)

		require.NoError(t, err)
		assert.Empty(t, internal.processed)
	})

	t.Run("skipsWhenShouldProcessFalse", func(t *testing.T) {
		storage := &statusRecorder{}
		internal := &fakeProcessor{shouldProcess: false}
		mw := NewProcessorMW(storage, internal)

		tr := testTransfer(t)
		_, err := mw.Process(ctx, tr)

		require.NoError(t, err)
		assert.Empty(t, storage.updates)
		assert.Equal(t, store.RelayStatusPending, tr.Status)
	})

	t.Run("statusUpdateFailurePoisons", func(t *testing.T) {
		storage := &statusRecorder{err: errors.New("db down")}
		internal := &fakeProcessor{shouldProcess: true}
		mw := NewProcessorMW(storage, internal)

		tr := testTransfer(t)
		out, err := mw.Process(ctx, tr)

		require.NoError(t, err)
		assert.NotEmpty(t, out.Error())
		assert.Empty(t, internal.processed)
		assert.Len(t, internal.cancelled, 1)
	})

	t.Run("processorErrorPoisonsButFlows", func(t *testing.T) {
		storage := &statusRecorder{}
		internal := &fakeProcessor{shouldProcess: true, processErr: errors.New("boom")}
		mw := NewProcessorMW(storage, internal)

		tr := testTransfer(t)
		out, err := mw.Process(ctx, tr)

		require.NoError(t, err)
		assert.Same(t, tr, out)
		assert.Equal(t, "boom", out.Error())
		assert.Len(t, internal.cancelled, 1)
	})
}

func TestBatchProcessorMW(t *testing.T) {
	ctx := context.Background()

	t.Run("partitionsBatch", func(t *testing.T) {
		storage := &statusRecorder{}
		applies := testTransfer(t)
		errored := testTransfer(t)
		errored.ProcessingError = errors.New("poisoned")

		internal := &fakeBatchProcessor{shouldProcess: func(tr *processors.Transfer) bool { return tr == applies }}
		mw := NewBatchProcessorMW(storage, internal)

		out, err := mw.Process(ctx, []*processors.Transfer{applies, errored})

		require.NoError(t, err)
		assert.Len(t, out, 2)
		require.Len(t, internal.processed, 1)
		assert.Equal(t, []*processors.Transfer{applies}, internal.processed[0])
	})

	t.Run("statusUpdateFailureExcludesFromBatch", func(t *testing.T) {
		storage := &statusRecorder{err: errors.New("db down")}
		internal := &fakeBatchProcessor{shouldProcess: func(*processors.Transfer) bool { return true }}
		mw := NewBatchProcessorMW(storage, internal)

		tr := testTransfer(t)
		out, err := mw.Process(ctx, []*processors.Transfer{tr})

		require.NoError(t, err)
		assert.Len(t, out, 1)
		assert.NotEmpty(t, tr.Error())
		// the transfer whose status update failed must not be processed
		require.Len(t, internal.processed, 1)
		assert.Empty(t, internal.processed[0])
	})

	t.Run("processErrorPoisonsProcessedOnly", func(t *testing.T) {
		storage := &statusRecorder{}
		applies := testTransfer(t)
		skipped := testTransfer(t)

		internal := &fakeBatchProcessor{
			shouldProcess: func(tr *processors.Transfer) bool { return tr == applies },
			processErr:    errors.New("boom"),
		}
		mw := NewBatchProcessorMW(storage, internal)

		out, err := mw.Process(ctx, []*processors.Transfer{applies, skipped})

		require.NoError(t, err)
		assert.Len(t, out, 2)
		assert.Equal(t, "boom", applies.Error())
		assert.Empty(t, skipped.Error())
	})
}
