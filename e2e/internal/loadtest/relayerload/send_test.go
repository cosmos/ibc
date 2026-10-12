// SPDX-License-Identifier: Apache-2.0

package relayerload

import (
	"context"
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/harness/traffic"
)

func TestPaceCountsUnusedTokensAsBackpressure(t *testing.T) {
	tokens := make(chan struct{}, 3)
	ticks := make(chan time.Time)
	var remaining atomic.Int64
	remaining.Store(100)
	var exhausted atomic.Bool
	done := make(chan int)
	go func() { done <- pace(t.Context(), ticks, nil, nil, tokens, 1, remaining.Load, exhausted.Load) }()

	// Interval 1: the first fill happens at once; one token is used.
	require.Eventually(t, func() bool { return len(tokens) == 3 }, time.Second, time.Millisecond)
	<-tokens
	// Interval 2: the two leftovers are backpressure and the bucket refills to 3.
	ticks <- time.Now()
	require.Eventually(t, func() bool { return len(tokens) == 3 }, time.Second, time.Millisecond)
	// Every slot is claimed: leftovers are not backpressure, and none are added.
	remaining.Store(0)
	ticks <- time.Now()
	require.Eventually(t, func() bool { return len(tokens) == 0 }, time.Second, time.Millisecond)
	// Every packet is sent: the next tick ends pacing.
	exhausted.Store(true)
	ticks <- time.Now()

	assert.Equal(t, 2, <-done)
}

// At the tail of a count run, unused tokens offer more transfers than are
// left to send; only those left count.
func TestPaceCapsBackpressureAtTheUnclaimedSlots(t *testing.T) {
	tokens := make(chan struct{}, 3)
	ticks := make(chan time.Time)
	var exhausted atomic.Bool
	remaining := func() int64 { return 4 }
	done := make(chan int)
	go func() { done <- pace(t.Context(), ticks, nil, nil, tokens, 5, remaining, exhausted.Load) }()

	require.Eventually(t, func() bool { return len(tokens) == 3 }, time.Second, time.Millisecond)
	// One token is used, so the refill after the tick shows.
	<-tokens
	ticks <- time.Now()
	require.Eventually(t, func() bool { return len(tokens) == 3 }, time.Second, time.Millisecond)
	exhausted.Store(true)
	ticks <- time.Now()

	assert.Equal(t, 4, <-done, "2 tokens of 5 transfers, but only 4 left to send")
}

func TestPaceStopsWhenEverySenderRetired(t *testing.T) {
	quit := make(chan struct{})
	close(quit)
	never := func() bool { return false }
	slots := func() int64 { return 10 }
	assert.Zero(t, pace(t.Context(), nil, quit, nil, make(chan struct{}, 2), 1, slots, never))
}

func TestPaceEndsAtTheDeadline(t *testing.T) {
	deadline := make(chan time.Time, 1)
	deadline <- time.Now()
	never := func() bool { return false }
	slots := func() int64 { return 10 }
	assert.Zero(t, pace(t.Context(), nil, nil, deadline, make(chan struct{}, 3), 1, slots, never))
}

// fakeTicks ticks every interval until the test ends.
func fakeTicks(t *testing.T, interval time.Duration) <-chan time.Time {
	ticker := time.NewTicker(interval)
	t.Cleanup(ticker.Stop)
	return ticker.C
}

func TestGenerateSendsTargetWithOneTransactionPerWallet(t *testing.T) {
	const wallets, target = 4, 20
	var inFlight [wallets]atomic.Int32
	var block atomic.Uint64
	var sequence atomic.Uint64
	send := func(_ context.Context, wallet, n int) ([]sentPacket, error) {
		if inFlight[wallet].Add(1) != 1 {
			t.Errorf("wallet %d has two transactions in flight", wallet)
		}
		defer inFlight[wallet].Add(-1)
		time.Sleep(2 * time.Millisecond)
		return packets(&sequence, block.Load(), n), nil
	}
	r := &DirectionReport{Route: "test", Target: target}
	out := &inbox{}
	ticks := make(chan time.Time)
	go func() {
		for range 100 {
			time.Sleep(10 * time.Millisecond)
			block.Add(1)
			select {
			case ticks <- time.Now():
			case <-t.Context().Done():
				return
			}
		}
	}()

	generate(t.Context(), ticks, pacing{perBlock: 3, batch: 1, wallets: wallets, limit: target}, send, out, r)

	received, closed := out.take()
	assert.True(t, closed)
	assert.Len(t, received, target)
	assert.Equal(t, target, r.Sent)
	assert.Zero(t, r.SendFailures)
	assert.Equal(t, target, r.SubmitToReceipt.Count)
	assert.Positive(t, r.BlocksWithTxs)
	assert.GreaterOrEqual(t, r.BlockSpan, r.BlocksWithTxs)
}

func TestGenerateRetiresFailingWallet(t *testing.T) {
	const wallets, target = 3, 12
	var calls [wallets]atomic.Int32
	var sequence atomic.Uint64
	send := func(_ context.Context, wallet, n int) ([]sentPacket, error) {
		calls[wallet].Add(1)
		if wallet == 0 {
			return nil, errors.New("nonce too low")
		}
		return packets(&sequence, 1, n), nil
	}
	r := &DirectionReport{Route: "test", Target: target}

	generate(t.Context(), fakeTicks(t, time.Millisecond),
		pacing{perBlock: wallets, batch: 1, wallets: wallets, limit: target}, send, &inbox{}, r)

	assert.Equal(t, int32(1), calls[0].Load(), "a failed wallet sends no more")
	assert.Equal(t, 1, r.SendFailures)
	assert.Equal(t, target, r.Sent, "another wallet sends the failed packet")
	assert.Equal(t, int32(target), calls[1].Load()+calls[2].Load())
}

func TestGenerateEndsWhenEveryWalletRetired(t *testing.T) {
	send := func(context.Context, int, int) ([]sentPacket, error) { return nil, errors.New("insufficient funds") }
	r := &DirectionReport{Route: "test", Target: 100}

	finished := make(chan struct{})
	go func() {
		generate(t.Context(), fakeTicks(t, time.Millisecond), pacing{perBlock: 5, batch: 1, wallets: 2, limit: 100},
			send, &inbox{}, r)
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("generate did not return after every wallet retired")
	}
	assert.Equal(t, 2, r.SendFailures)
	assert.Zero(t, r.Sent)
}

func TestGenerateWithALimitOfZeroFinishesAtOnce(t *testing.T) {
	send := func(context.Context, int, int) ([]sentPacket, error) {
		t.Error("a direction with nothing to send sent")
		return nil, nil
	}
	out := &inbox{}
	r := &DirectionReport{Route: "test"}

	// No tick ever comes: generate must not wait for one.
	generate(t.Context(), nil, pacing{perBlock: 5, batch: 1, wallets: 2}, send, out, r)

	received, closed := out.take()
	assert.True(t, closed)
	assert.Empty(t, received)
	assert.Zero(t, r.Backpressure)
}

func TestBlockCounts(t *testing.T) {
	blocks, span, low, median, high := blockCounts(map[uint64]int{10: 5, 11: 1, 14: 3})
	assert.Equal(t, []int{3, 5, 1, 3, 5}, []int{blocks, span, low, median, high})

	blocks, span, low, median, high = blockCounts(nil)
	assert.Equal(t, []int{0, 0, 0, 0, 0}, []int{blocks, span, low, median, high})
}

// The sender waits for its receipt; a canceled run must not leave senders
// behind, nor count the interrupted transfers as failures.
func TestGenerateStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	var once sync.Once
	send := func(ctx context.Context, _, _ int) ([]sentPacket, error) {
		once.Do(cancel)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	r := &DirectionReport{Route: "test", Target: 10}
	generate(ctx, fakeTicks(t, time.Millisecond), pacing{perBlock: 2, batch: 1, wallets: 2, limit: 10}, send,
		&inbox{}, r)
	assert.Zero(t, r.Sent)
	assert.Zero(t, r.SendFailures)
}

// packets returns n packets with the next sequences, included in block.
func packets(sequence *atomic.Uint64, block uint64, n int) []sentPacket {
	out := make([]sentPacket, n)
	for k := range out {
		out[k] = sentPacket{key: packetKey{"client-0", sequence.Add(1)}, block: block}
	}
	return out
}

func TestGenerateSendsTheTargetInBatchesWithAShortLast(t *testing.T) {
	var sizes []int
	var mu sync.Mutex
	var sequence atomic.Uint64
	send := func(_ context.Context, _, n int) ([]sentPacket, error) {
		mu.Lock()
		sizes = append(sizes, n)
		mu.Unlock()
		return packets(&sequence, 1, n), nil
	}
	r := &DirectionReport{Route: "test", Target: 12}
	out := &inbox{}

	generate(t.Context(), fakeTicks(t, time.Millisecond), pacing{perBlock: 10, batch: 5, wallets: 1, limit: 12}, send,
		out, r)

	received, _ := out.take()
	assert.Equal(t, []int{5, 5, 2}, sizes)
	assert.Equal(t, 12, r.Sent)
	assert.Len(t, received, 12)
	assert.Equal(t, 3, r.SubmitToReceipt.Count, "one sample per transaction")
}

func TestGenerateCompletesTheSendsInFlightAtTheDeadline(t *testing.T) {
	var sequence atomic.Uint64
	release := make(chan struct{})
	var inFlight atomic.Int32
	send := func(_ context.Context, _, n int) ([]sentPacket, error) {
		inFlight.Add(1)
		<-release
		return packets(&sequence, 1, n), nil
	}
	deadline := time.Now().Add(200 * time.Millisecond)
	r := &DirectionReport{Route: "test"}
	finished := make(chan struct{})
	go func() {
		generate(t.Context(), fakeTicks(t, time.Millisecond), pacing{
			perBlock: 4, batch: 2, wallets: 3, limit: math.MaxInt64, deadline: deadline,
		}, send, &inbox{}, r)
		close(finished)
	}()

	// Every wallet has a transaction in flight; the deadline passes meanwhile.
	require.Eventually(t, func() bool { return inFlight.Load() == 3 }, time.Second, time.Millisecond)
	time.Sleep(time.Until(deadline) + 20*time.Millisecond)
	close(release)
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("generate did not return after the deadline")
	}
	assert.Equal(t, int32(3), inFlight.Load(), "no transaction starts after the deadline")
	assert.Equal(t, 6, r.Sent, "the transactions in flight at the deadline still count")
	assert.Zero(t, r.SendFailures)
}

// Senders that take a token once the deadline passed start no send, though
// pace filled the bucket before it saw the deadline.
func TestGenerateStartsNoSendAfterTheDeadline(t *testing.T) {
	var calls atomic.Int32
	send := func(context.Context, int, int) ([]sentPacket, error) {
		calls.Add(1)
		return nil, nil
	}
	r := &DirectionReport{Route: "test"}

	generate(t.Context(), fakeTicks(t, time.Millisecond), pacing{
		perBlock: 8, batch: 1, wallets: 8, limit: math.MaxInt64, deadline: time.Now(),
	}, send, &inbox{}, r)

	assert.Zero(t, calls.Load())
	assert.Zero(t, r.SendFailures)
}

func TestSendDispatchesOnTheBatchSize(t *testing.T) {
	var plain, batched []int
	transfer := func(context.Context, int) (traffic.SendResult, error) {
		plain = append(plain, 1)
		return traffic.SendResult{}, nil
	}
	transferBatch := func(_ context.Context, _, n int) ([]traffic.SendResult, error) {
		batched = append(batched, n)
		return make([]traffic.SendResult, n), nil
	}

	_, err := newSend(1, transfer, transferBatch)(t.Context(), 0, 1)
	require.NoError(t, err)
	sent, err := newSend(5, transfer, transferBatch)(t.Context(), 0, 1)
	require.NoError(t, err)

	assert.Equal(t, []int{1}, plain)
	assert.Equal(t, []int{1}, batched, "a batched run's short last transaction still goes through the shim")
	assert.Len(t, sent, 1)
}
