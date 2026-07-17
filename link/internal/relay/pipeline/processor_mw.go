package pipeline

import (
	"context"

	"github.com/deliveryhero/pipeline/v2"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/relay/processors"
	"github.com/cosmos/ibc/link/internal/store"
)

// StatusStorage persists packet status transitions.
type StatusStorage interface {
	UpdatePacketStatus(ctx context.Context, key store.PacketKey, status store.RelayStatus) error
}

// Processor a pipeline step for a tr; wrapped by ProcessorMW.
type Processor interface {
	// ShouldProcess reports whether this processor applies to the tr.
	ShouldProcess(tr *processors.Transfer) bool

	// Status the status a tr is in while processed by this processor.
	Status() store.RelayStatus

	pipeline.Processor[*processors.Transfer, *processors.Transfer]
}

// ProcessorMW wraps a Processor with the shared tr handling: errored,
// failed, and non-applicable transfers pass through untouched; the status
// transition is persisted before processing; processor errors poison the
// tr instead of dropping it so it still reaches the pipeline output.
type ProcessorMW struct {
	storage  StatusStorage
	internal Processor
}

func NewProcessorMW(storage StatusStorage, internal Processor) ProcessorMW {
	return ProcessorMW{storage: storage, internal: internal}
}

func (mw ProcessorMW) Process(ctx context.Context, input *processors.Transfer) (*processors.Transfer, error) {
	// input may be nil while the pipeline channels close down
	if input == nil || input.Error() != "" {
		return input, nil
	}

	if input.Status == store.RelayStatusFailed {
		return input, nil
	}

	if !mw.internal.ShouldProcess(input) {
		return input, nil
	}

	if err := mw.storage.UpdatePacketStatus(ctx, input.Key(), mw.internal.Status()); err != nil {
		wrapped := errors.Wrapf(err, "updating packet status to %s from %s", mw.internal.Status(), input.Status)
		mw.Cancel(input, wrapped)
		input.ProcessingError = wrapped

		return input, nil
	}

	prevStatus := input.Status
	input.Status = mw.internal.Status()

	input.GetLogger().Debug("Processing transfer", "status", input.Status, "prevStatus", prevStatus)

	output, err := mw.internal.Process(ctx, input)
	if err != nil {
		// on shutdown the outer processor cancels for us
		if errors.Is(err, context.Canceled) {
			return nil, err
		}

		mw.Cancel(input, err)
		input.ProcessingError = err

		return input, nil
	}

	input.GetLogger().Debug("Processed transfer", "status", input.Status, "prevStatus", prevStatus)

	return output, nil
}

func (mw ProcessorMW) Cancel(input *processors.Transfer, err error) {
	if input == nil {
		return
	}

	mw.internal.Cancel(input, err)
}

// BatchProcessor a pipeline step for a batch of transfers; wrapped by
// BatchProcessorMW.
type BatchProcessor interface {
	ShouldProcess(tr *processors.Transfer) bool
	Status() store.RelayStatus

	pipeline.Processor[[]*processors.Transfer, []*processors.Transfer]
}

// BatchProcessorMW the batch equivalent of ProcessorMW: it partitions the
// batch into transfers this processor applies to and pass-throughs, persists
// status transitions, and poisons rather than drops transfers on errors.
type BatchProcessorMW struct {
	storage  StatusStorage
	internal BatchProcessor
}

func NewBatchProcessorMW(storage StatusStorage, internal BatchProcessor) BatchProcessorMW {
	return BatchProcessorMW{storage: storage, internal: internal}
}

func (mw BatchProcessorMW) Process(ctx context.Context, batch []*processors.Transfer) ([]*processors.Transfer, error) {
	var notProcessing []*processors.Transfer
	var toProcess []*processors.Transfer

	for _, input := range batch {
		switch {
		case input.Error() != "", input.Status == store.RelayStatusFailed, !mw.internal.ShouldProcess(input):
			// erroring here would fail the whole batch; pass these through
			notProcessing = append(notProcessing, input)

			continue
		}

		if err := mw.storage.UpdatePacketStatus(ctx, input.Key(), mw.internal.Status()); err != nil {
			wrapped := errors.Wrapf(err, "updating packet status to %s from %s", mw.internal.Status(), input.Status)
			mw.Cancel([]*processors.Transfer{input}, wrapped)
			input.ProcessingError = wrapped
			notProcessing = append(notProcessing, input)

			continue
		}

		input.Status = mw.internal.Status()
		toProcess = append(toProcess, input)
	}

	output, err := mw.internal.Process(ctx, toProcess)
	if err != nil {
		// on shutdown the outer processor cancels for us
		if errors.Is(err, context.Canceled) {
			return nil, err
		}

		mw.Cancel(toProcess, err)
		for _, input := range toProcess {
			input.ProcessingError = err
		}

		return append(toProcess, notProcessing...), nil
	}

	return append(output, notProcessing...), nil
}

func (mw BatchProcessorMW) Cancel(batch []*processors.Transfer, err error) {
	mw.internal.Cancel(batch, err)
}

func (mw BatchProcessorMW) ShouldProcess(input *processors.Transfer) bool {
	return mw.internal.ShouldProcess(input)
}

func (mw BatchProcessorMW) Status() store.RelayStatus {
	return mw.internal.Status()
}
