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

// sendFunc submits one transfer from wallet and waits for its receipt.
type sendFunc func(ctx context.Context, wallet int) (sentPacket, error)

// generate sends up to r.Target packets from one sender goroutine per wallet.
// Each tick of ticks opens a pacing interval of perBlock tokens; a sender
// takes a token, claims a packet slot, and sends, so every wallet has at most
// one transaction in flight. A sender whose transfer fails is retired. Every
// sent packet goes to out, which generate closes when it returns; out must
// have room for r.Target packets or be drained concurrently.
func generate(
	ctx context.Context,
	ticks <-chan time.Time,
	perBlock, wallets int,
	send sendFunc,
	out chan<- sentPacket,
	r *DirectionReport,
) {
	defer close(out)
	tokens := make(chan struct{}, perBlock)
	var claimed atomic.Int64
	target := int64(r.Target)
	claim := func() bool { return claimed.Add(1) <= target }
	exhausted := func() bool { return claimed.Load() >= target }

	var (
		mu        sync.Mutex
		samples   []time.Duration
		perBlocks = map[uint64]int{}
		started   = time.Now()
	)
	record := func(packet sentPacket, submitted time.Time, err error) {
		now := time.Now()
		mu.Lock()
		defer mu.Unlock()
		switch {
		case err != nil && ctx.Err() != nil:
			// Interrupted, not a failed transfer.
			return
		case err != nil:
			r.SendFailures++
			slog.WarnContext(ctx, "transfer failed; retiring wallet", "direction", r.Route, "err", err)
			return
		}
		r.Sent++
		samples = append(samples, now.Sub(submitted))
		perBlocks[packet.block]++
		r.lastSend = now
		out <- packet
	}

	stop := make(chan struct{})
	var senders sync.WaitGroup
	for wallet := range wallets {
		senders.Go(func() {
			for {
				select {
				case <-ctx.Done():
					return
				case <-stop:
					return
				case <-tokens:
				}
				if !claim() {
					return
				}
				submitted := time.Now()
				packet, err := send(ctx, wallet)
				record(packet, submitted, err)
				if err != nil {
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

	r.Ticks, r.Backpressure = pace(ctx, ticks, retired, tokens, exhausted)
	close(stop)
	senders.Wait()

	r.SubmitToReceipt = stats.Summarize(samples)
	if !r.lastSend.IsZero() {
		r.SendPhaseMS = r.lastSend.Sub(started).Milliseconds()
	}
	r.BlocksWithTxs, r.BlockSpan, r.PerBlockMin, r.PerBlockP50, r.PerBlockMax = blockCounts(perBlocks)
}

// pace fills tokens with cap(tokens) tokens now and on every tick, until
// exhausted reports that no packet slots remain, quit closes, or ctx ends.
// Tokens still unused when the next tick arrives count as backpressure: no
// wallet was free to send them within their interval. They are discarded so
// an interval never offers more than cap(tokens) sends.
func pace(
	ctx context.Context,
	ticks <-chan time.Time,
	quit <-chan struct{},
	tokens chan struct{},
	exhausted func() bool,
) (ticked, backpressure int) {
	for {
		for drained := false; !drained; {
			select {
			case <-tokens:
				backpressure++
			default:
				drained = true
			}
		}
		for range cap(tokens) {
			tokens <- struct{}{}
		}
		ticked++
		select {
		case <-ctx.Done():
			return ticked, backpressure
		case <-quit:
			return ticked, backpressure
		case <-ticks:
		}
		if exhausted() {
			return ticked, backpressure
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
