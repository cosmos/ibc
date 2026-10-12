// SPDX-License-Identifier: Apache-2.0

package relayerload

import (
	"context"
	"log/slog"
	"maps"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cosmos/ibc/e2e/internal/loadtest/stats"
)

// packetKey identifies a packet as the relayer does.
type packetKey struct {
	clientID string
	sequence uint64
}

// sentPacket is one packet a sender's transfer emitted.
type sentPacket struct {
	key packetKey
	// block is the source block that included the transfer.
	block uint64
}

// sendFunc submits one transaction of n transfers from wallet, waits for
// its receipt, and returns the packets it emitted.
type sendFunc func(ctx context.Context, wallet, n int) ([]sentPacket, error)

// inbox carries sent packets from the senders to the sweep without ever
// holding up a sender.
type inbox struct {
	mu      sync.Mutex
	packets []sentPacket
	closed  bool
}

func (b *inbox) put(packets []sentPacket) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.packets = append(b.packets, packets...)
}

func (b *inbox) close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
}

// take returns the packets put since the last take, and whether the inbox
// was closed, so that no packet follows them.
func (b *inbox) take() ([]sentPacket, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	packets := b.packets
	b.packets = nil
	return packets, b.closed
}

// pacing shapes how one direction sends.
type pacing struct {
	// perBlock is the transfers offered per interval, in transactions of
	// batch transfers; batch divides it.
	perBlock, batch int
	wallets         int
	// limit is the most packets to send: the target of a count run, or
	// math.MaxInt64 to send until the deadline.
	limit int64
	// deadline, when not zero, ends sending: no send starts after it.
	deadline time.Time
}

// generate sends from one sender goroutine per wallet until p.limit packets
// are sent or p.deadline passes. Each tick of ticks opens a pacing interval
// of p.perBlock/p.batch tokens; a sender takes a token, claims up to p.batch
// packet slots, and sends them in one transaction, so every wallet has at
// most one transaction in flight. A sender whose transaction fails is
// retired and hands its slots back for another wallet to send, so the limit
// falls short only if every wallet retires. Sends in flight at the deadline
// complete. Every sent packet goes to out, which generate closes when it
// returns.
func generate(
	ctx context.Context,
	ticks <-chan time.Time,
	p pacing,
	send sendFunc,
	out *inbox,
	r *DirectionReport,
) {
	defer out.close()
	tokens := make(chan struct{}, p.perBlock/p.batch)
	var claimed atomic.Int64
	batch := int64(p.batch)
	// claim takes up to a batch of the unclaimed packet slots.
	claim := func() int64 {
		for {
			c := claimed.Load()
			n := min(batch, p.limit-c)
			if n <= 0 {
				return 0
			}
			if claimed.CompareAndSwap(c, c+n) {
				return n
			}
		}
	}
	remaining := func() int64 { return p.limit - claimed.Load() }
	expired := func() bool { return !p.deadline.IsZero() && !time.Now().Before(p.deadline) }

	var (
		mu        sync.Mutex
		samples   []time.Duration
		perBlocks = map[uint64]int{}
		started   = time.Now()
	)
	exhausted := func() bool {
		mu.Lock()
		defer mu.Unlock()
		return int64(r.Sent) >= p.limit
	}
	record := func(packets []sentPacket, submitted time.Time, err error) {
		now := time.Now()
		mu.Lock()
		defer mu.Unlock()
		switch {
		case err != nil && ctx.Err() != nil:
			// Interrupted, not a failed transfer.
			return
		case err != nil:
			r.SendFailures++
			if r.FirstSendError == "" {
				r.FirstSendError = err.Error()
			}
			slog.WarnContext(ctx, "transfer failed; retiring wallet", "direction", r.Route, "err", err)
			return
		}
		r.Sent += len(packets)
		samples = append(samples, now.Sub(submitted))
		r.lastSend = now
		for _, packet := range packets {
			perBlocks[packet.block]++
		}
		out.put(packets)
	}

	stop := make(chan struct{})
	var senders sync.WaitGroup
	for wallet := range p.wallets {
		senders.Go(func() {
			for {
				select {
				case <-ctx.Done():
					return
				case <-stop:
					return
				case <-tokens:
				}
				if expired() {
					return
				}
				n := claim()
				if n == 0 {
					// Every slot is taken, but a failing send may hand some back.
					continue
				}
				submitted := time.Now()
				packets, err := send(ctx, wallet, int(n))
				record(packets, submitted, err)
				if err != nil {
					claimed.Add(-n)
					return
				}
			}
		})
	}
	retired := make(chan struct{})
	go func() {
		senders.Wait()
		close(retired)
	}()

	var deadline <-chan time.Time
	if !p.deadline.IsZero() {
		timer := time.NewTimer(time.Until(p.deadline))
		defer timer.Stop()
		deadline = timer.C
	}
	r.Backpressure = pace(ctx, ticks, retired, deadline, tokens, batch, remaining, exhausted)
	close(stop)
	senders.Wait()

	r.SubmitToReceipt = stats.Summarize(samples)
	if !r.lastSend.IsZero() {
		r.SendPhaseMS = r.lastSend.Sub(started).Milliseconds()
	}
	r.BlocksWithTxs, r.BlockSpan, r.PerBlockMin, r.PerBlockP50, r.PerBlockMax = blockCounts(perBlocks)
}

// pace fills tokens with cap(tokens) tokens now and on every tick while
// remaining reports unclaimed packet slots, until exhausted reports that
// every packet was sent, quit closes, deadline fires, or ctx ends. Tokens
// still unused when the next tick arrives count as backpressure, in
// transfers of batch each and at most the slots still unclaimed: no wallet
// was free to send them within their interval. They are discarded so an
// interval never offers more than cap(tokens) sends.
func pace(
	ctx context.Context,
	ticks <-chan time.Time,
	quit <-chan struct{},
	deadline <-chan time.Time,
	tokens chan struct{},
	batch int64,
	remaining func() int64,
	exhausted func() bool,
) (backpressure int) {
	for !exhausted() {
		var unused int64
		for drained := false; !drained; {
			select {
			case <-tokens:
				unused++
			default:
				drained = true
			}
		}
		left := remaining()
		backpressure += int(min(unused*batch, left))
		if left > 0 {
			for range cap(tokens) {
				tokens <- struct{}{}
			}
		}
		select {
		case <-ctx.Done():
			return backpressure
		case <-quit:
			return backpressure
		case <-deadline:
			// Senders check the deadline after taking a token, so the
			// tokens left behind start no send.
			return backpressure
		case <-ticks:
		}
	}
	return backpressure
}

// blockCounts summarizes how many sends each including block held: the
// blocks with at least one, the span from the first to the last such block,
// and the min, median, and max count over the blocks with at least one.
func blockCounts(perBlock map[uint64]int) (blocks, span, low, median, high int) {
	if len(perBlock) == 0 {
		return 0, 0, 0, 0, 0
	}
	numbers := slices.Sorted(maps.Keys(perBlock))
	counts := slices.Sorted(maps.Values(perBlock))
	span = int(numbers[len(numbers)-1]-numbers[0]) + 1
	return len(counts), span, counts[0], counts[(len(counts)-1)/2], counts[len(counts)-1]
}
