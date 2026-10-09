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

// pacing shapes how one direction sends.
type pacing struct {
	// perBlock is the transfers offered per interval, in transactions of
	// batch transfers; batch divides it.
	perBlock, batch int
	wallets         int
	// window, when not nil, ends sending when it fires.
	window <-chan time.Time
}

// generate sends from one sender goroutine per wallet until r.Target packets
// are sent or, with no target, until p.window fires. Each tick of ticks opens
// a pacing interval of p.perBlock/p.batch tokens; a sender takes a token,
// claims up to p.batch packet slots, and sends them in one transaction, so
// every wallet has at most one transaction in flight. A sender whose
// transaction fails is retired and hands its slots back for another wallet
// to send, so the target falls short only if every wallet retires. Sends in
// flight when the window ends complete. Every sent packet goes to out, which
// generate closes when it returns; out must have room for every packet sent
// or be drained concurrently.
func generate(
	ctx context.Context,
	ticks <-chan time.Time,
	p pacing,
	send sendFunc,
	out chan<- sentPacket,
	r *DirectionReport,
) {
	defer close(out)
	tokens := make(chan struct{}, p.perBlock/p.batch)
	var claimed, sent atomic.Int64
	target := int64(r.Target)
	batch := int64(p.batch)
	// claim takes up to a batch of the unclaimed packet slots.
	claim := func() int64 {
		for {
			c := claimed.Load()
			n := batch
			if target > 0 {
				n = min(batch, target-c)
			}
			if n <= 0 {
				return 0
			}
			if claimed.CompareAndSwap(c, c+n) {
				return n
			}
		}
	}
	open := func() bool { return target == 0 || claimed.Load() < target }
	exhausted := func() bool { return target > 0 && sent.Load() >= target }

	var (
		mu        sync.Mutex
		samples   []time.Duration
		perBlocks = map[uint64]int{}
		started   = time.Now()
	)
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
		sent.Add(int64(len(packets)))
		samples = append(samples, now.Sub(submitted))
		r.lastSend = now
		for _, packet := range packets {
			perBlocks[packet.block]++
			out <- packet
		}
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

	r.Backpressure = p.batch * pace(ctx, ticks, retired, p.window, tokens, open, exhausted)
	close(stop)
	senders.Wait()

	r.SubmitToReceipt = stats.Summarize(samples)
	if !r.lastSend.IsZero() {
		r.SendPhaseMS = r.lastSend.Sub(started).Milliseconds()
	}
	r.BlocksWithTxs, r.BlockSpan, r.PerBlockMin, r.PerBlockP50, r.PerBlockMax = blockCounts(perBlocks)
}

// pace fills tokens with cap(tokens) tokens now and on every tick while open
// reports unclaimed packet slots, until exhausted reports that every packet
// was sent, quit closes, window fires, or ctx ends. Tokens still unused when
// the next tick arrives, with slots still open, count as backpressure: no
// wallet was free to send them within their interval. They are discarded so
// an interval never offers more than cap(tokens) sends.
func pace(
	ctx context.Context,
	ticks <-chan time.Time,
	quit <-chan struct{},
	window <-chan time.Time,
	tokens chan struct{},
	open, exhausted func() bool,
) (backpressure int) {
	for {
		unclaimed := open()
		for drained := false; !drained; {
			select {
			case <-tokens:
				if unclaimed {
					backpressure++
				}
			default:
				drained = true
			}
		}
		if unclaimed {
			for range cap(tokens) {
				tokens <- struct{}{}
			}
		}
		select {
		case <-ctx.Done():
			return backpressure
		case <-quit:
			return backpressure
		case <-window:
			// No send starts after the window: its unused tokens go.
			for drained := false; !drained; {
				select {
				case <-tokens:
				default:
					drained = true
				}
			}
			return backpressure
		case <-ticks:
		}
		if exhausted() {
			return backpressure
		}
	}
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
