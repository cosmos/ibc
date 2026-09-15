// SPDX-License-Identifier: Apache-2.0

package watcher

import (
	"context"
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

// fakeChain models what one chain has sent and what is still committed at a
// given height, and stands in for the event stream a watcher subscribes to.
type fakeChain struct {
	t  *testing.T
	mu sync.Mutex

	head   uint64
	sentAt map[uint64]uint64

	// headReads are served to successive latest-header reads before head is,
	// so a test can hand the pass a head that moves backwards
	headReads []uint64

	settled   map[uint64]struct{}
	pruned    map[uint64]struct{}
	latestErr error
	headErr   error
	probeErr  error
	passes    int
	probes    [][]uint64
	heights   []uint64
	finds     [][]uint64
	gate      chan struct{}

	subs        []*subscription
	subFailWith error
}

// subscription is one opened stream, which the test drives as the chain would.
type subscription struct {
	clientIDs    []string
	ctx          context.Context //nolint:containedctx // the test asserts on its cancellation
	out          chan<- v2.PacketEvent
	errs         chan error
	unsubscribed bool
}

func newFakeChain(t *testing.T) *fakeChain {
	t.Helper()

	return &fakeChain{
		t:       t,
		sentAt:  make(map[uint64]uint64),
		settled: make(map[uint64]struct{}),
		pruned:  make(map[uint64]struct{}),
	}
}

func (c *fakeChain) GetBlockHeader(_ context.Context, height uint64) (v2.BlockHeader, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.headErr != nil {
		return v2.BlockHeader{}, c.headErr
	}

	if height == v2.LatestBlock {
		height = c.head

		if len(c.headReads) > 0 {
			height, c.headReads = c.headReads[0], c.headReads[1:]
		}
	}

	return v2.BlockHeader{Height: height, Timestamp: blockTime}, nil
}

// LatestPacketSequence answers for the height it is asked about, which is what
// makes a pinned height mean anything. Every pass reads it exactly once.
func (c *fakeChain) LatestPacketSequence(_ context.Context, _ string, height uint64) (uint64, error) {
	c.mu.Lock()

	c.passes++
	latest, err, gate := c.sequenceAt(height), c.latestErr, c.gate
	c.mu.Unlock()

	if gate != nil {
		<-gate
	}

	return latest, err
}

// PacketCommitments answers as a node at height would: a send assigned above it
// has no commitment there yet.
func (c *fakeChain) PacketCommitments(
	_ context.Context,
	_ string,
	sequences []uint64,
	height uint64,
) ([]uint64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.probes = append(c.probes, slices.Clone(sequences))
	c.heights = append(c.heights, height)

	if c.probeErr != nil {
		return nil, c.probeErr
	}

	var live []uint64

	for _, sequence := range sequences {
		if _, gone := c.settled[sequence]; c.sentBy(sequence, height) && !gone {
			live = append(live, sequence)
		}
	}

	return live, nil
}

func (c *fakeChain) FindSendPackets(_ context.Context, _ string, sequences []uint64) ([]v2.PacketEvent, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.finds = append(c.finds, slices.Clone(sequences))

	var events []v2.PacketEvent

	for _, sequence := range sequences {
		if _, gone := c.pruned[sequence]; c.sentSequences(sequence) && !gone {
			events = append(events, sendPacketEvent(sequence))
		}
	}

	return events, nil
}

func (c *fakeChain) SubscribeSendPackets(
	ctx context.Context,
	clientIDs []string,
	out chan<- v2.PacketEvent,
) (v2.Subscription, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.subFailWith; err != nil {
		c.subFailWith = nil

		return nil, err
	}

	sub := &subscription{clientIDs: clientIDs, ctx: ctx, out: out, errs: make(chan error, 1)}
	c.subs = append(c.subs, sub)

	return sub, nil
}

func (s *subscription) Err() <-chan error { return s.errs }

func (s *subscription) Unsubscribe() { s.unsubscribed = true }

// sendSequences records sequences as sent at the current head and still outstanding.
func (c *fakeChain) sendSequences(sequences ...uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, sequence := range sequences {
		c.sentAt[sequence] = c.head
	}
}

// mineBlocks advances the head, so sends after it read as assigned above an earlier height.
func (c *fakeChain) mineBlocks(blocks uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.head += blocks
}

// sequenceAt is the highest sequence assigned at or below height.
func (c *fakeChain) sequenceAt(height uint64) uint64 {
	var latest uint64

	for sequence, at := range c.sentAt {
		if at <= height {
			latest = max(latest, sequence)
		}
	}

	return latest
}

func (c *fakeChain) sentBy(sequence, height uint64) bool {
	at, ok := c.sentAt[sequence]

	return ok && at <= height
}

func (c *fakeChain) sentSequences(sequence uint64) bool {
	_, ok := c.sentAt[sequence]

	return ok
}

// settleSequences deletes the packet commitments, as an ack or a timeout does.
func (c *fakeChain) settleSequences(sequences ...uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, sequence := range sequences {
		c.settled[sequence] = struct{}{}
	}
}

// pruneSequences drops sends from the log index while leaving their commitments live.
func (c *fakeChain) pruneSequences(sequences ...uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, sequence := range sequences {
		c.pruned[sequence] = struct{}{}
	}
}

// unpruneSequences serves the sends again, as pointing the relayer at an archive endpoint does.
func (c *fakeChain) unpruneSequences(sequences ...uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, sequence := range sequences {
		delete(c.pruned, sequence)
	}
}

// blockClears blocks every clearing pass until the returned func releases them. It
// ignores the context on purpose, so a test can assert Stop waits for a pass
// that has not noticed the cancellation yet.
func (c *fakeChain) blockClears() func() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.gate = make(chan struct{})
	gate := c.gate

	return func() { close(gate) }
}

func (c *fakeChain) failLatest(err error)   { c.mu.Lock(); c.latestErr = err; c.mu.Unlock() }
func (c *fakeChain) failHead(err error)     { c.mu.Lock(); c.headErr = err; c.mu.Unlock() }
func (c *fakeChain) failProbe(err error)    { c.mu.Lock(); c.probeErr = err; c.mu.Unlock() }
func (c *fakeChain) passCount() int         { c.mu.Lock(); defer c.mu.Unlock(); return c.passes }
func (c *fakeChain) probeCalls() [][]uint64 { c.mu.Lock(); defer c.mu.Unlock(); return c.probes }
func (c *fakeChain) findCalls() [][]uint64  { c.mu.Lock(); defer c.mu.Unlock(); return c.finds }
func (c *fakeChain) probeHeights() []uint64 { c.mu.Lock(); defer c.mu.Unlock(); return c.heights }

// serveHeads queues the heights successive latest-header reads report.
func (c *fakeChain) serveHeads(heights ...uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.headReads = heights
}

// failNextSub makes the next subscribe fail rather than open.
func (c *fakeChain) failNextSub(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.subFailWith = err
}

func (c *fakeChain) openedSubs() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.subs)
}

// latestSub is the subscription the watcher is currently reading from.
func (c *fakeChain) latestSub() *subscription {
	c.t.Helper()

	c.mu.Lock()
	defer c.mu.Unlock()

	require.NotEmpty(c.t, c.subs, "watcher has not subscribed")

	return c.subs[len(c.subs)-1]
}
