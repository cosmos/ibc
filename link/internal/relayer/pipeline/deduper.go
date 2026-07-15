package pipeline

import (
	"context"
	"sync"

	"github.com/cosmos/ibc/link/internal/store"
)

// Deduper rejects transfers already in flight in the wrapped pipeline.
// It consumes the pipeline's output itself to learn when transfers leave.
type Deduper struct {
	pipeline TransferPipeline

	mu         sync.Mutex
	inPipeline map[store.PacketKey]struct{}

	done chan struct{}
}

var _ TransferPipeline = (*Deduper)(nil)

func NewDeduper(pipeline TransferPipeline) *Deduper {
	deduper := &Deduper{
		pipeline:   pipeline,
		inPipeline: make(map[store.PacketKey]struct{}),
		done:       make(chan struct{}),
	}

	go deduper.drain()

	return deduper
}

func (d *Deduper) Push(ctx context.Context, transfer *Transfer) bool {
	d.mu.Lock()

	key := transfer.Key()
	if _, exists := d.inPipeline[key]; exists {
		d.mu.Unlock()

		return false
	}

	d.inPipeline[key] = struct{}{}
	d.mu.Unlock()

	return d.pipeline.Push(ctx, transfer)
}

// Poll is a noop: the deduper drains the wrapped pipeline itself.
func (d *Deduper) Poll() (*Transfer, error) {
	return nil, nil
}

func (d *Deduper) Close() {
	d.pipeline.Close()
	<-d.done
}

func (d *Deduper) drain() {
	defer close(d.done)

	for {
		transfer, err := d.pipeline.Poll()
		if err != nil {
			return
		}

		if transfer == nil {
			continue
		}

		d.mu.Lock()
		delete(d.inPipeline, transfer.Key())
		d.mu.Unlock()
	}
}
