// SPDX-License-Identifier: Apache-2.0

package dispatch

import (
	"context"
	"sync"

	"github.com/cosmos/ibc/cli/internal/relay/pipeline"
	"github.com/cosmos/ibc/cli/internal/relay/processors"
	"github.com/cosmos/ibc/cli/internal/store"
)

// Deduper rejects transfers already in flight in the wrapped pipeline.
// It consumes the pipeline's output itself to learn when transfers leave.
// A transfer that left stays rejected until Release: a packet read before
// it left may predate its last writes.
type Deduper struct {
	pipeline pipeline.TransferPipeline

	mu         sync.Mutex
	inPipeline map[store.PacketKey]struct{}
	// left holds the keys in inPipeline whose transfers have left
	left []store.PacketKey

	done chan struct{}
}

var _ pipeline.TransferPipeline = (*Deduper)(nil)

func NewDeduper(pl pipeline.TransferPipeline) *Deduper {
	deduper := &Deduper{
		pipeline:   pl,
		inPipeline: make(map[store.PacketKey]struct{}),
		done:       make(chan struct{}),
	}

	go deduper.drain()

	return deduper
}

func (d *Deduper) Push(ctx context.Context, tr *processors.Transfer) bool {
	d.mu.Lock()

	key := tr.Key()
	if _, exists := d.inPipeline[key]; exists {
		d.mu.Unlock()

		return false
	}

	d.inPipeline[key] = struct{}{}
	d.mu.Unlock()

	return d.pipeline.Push(ctx, tr)
}

// Release lets the transfers that have left the pipeline be pushed again.
// Call it before reading the packets to push.
func (d *Deduper) Release() {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, key := range d.left {
		delete(d.inPipeline, key)
	}

	d.left = nil
}

// Poll is a noop: the deduper drains the wrapped pipeline itself.
func (d *Deduper) Poll() (*processors.Transfer, error) {
	return nil, nil
}

func (d *Deduper) Close() {
	d.pipeline.Close()
	<-d.done
}

func (d *Deduper) drain() {
	defer close(d.done)

	for {
		tr, err := d.pipeline.Poll()
		if err != nil {
			return
		}

		if tr == nil {
			continue
		}

		d.mu.Lock()
		d.left = append(d.left, tr.Key())
		d.mu.Unlock()
	}
}
