// SPDX-License-Identifier: Apache-2.0

package relayerload

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPaceCountsUnusedTokensAsBackpressure(t *testing.T) {
	tokens := make(chan struct{}, 3)
	ticks := make(chan time.Time)
	var exhausted atomic.Bool
	type result struct{ ticked, backpressure int }
	done := make(chan result)
	go func() {
		ticked, backpressure := pace(t.Context(), ticks, nil, tokens, exhausted.Load)
		done <- result{ticked, backpressure}
	}()

	// Interval 1: the first fill happens at once; one token is used.
	require.Eventually(t, func() bool { return len(tokens) == 3 }, time.Second, time.Millisecond)
	<-tokens
	// Interval 2: the two leftovers are backpressure and the bucket refills to 3.
	ticks <- time.Now()
	require.Eventually(t, func() bool { return len(tokens) == 3 }, time.Second, time.Millisecond)
	// Slots run out: the next tick ends pacing without counting leftovers.
	exhausted.Store(true)
	ticks <- time.Now()

	got := <-done
	assert.Equal(t, result{ticked: 2, backpressure: 2}, got)
}

func TestPaceStopsWhenEverySenderRetired(t *testing.T) {
	quit := make(chan struct{})
	close(quit)
	ticked, backpressure := pace(t.Context(), nil, quit, make(chan struct{}, 2), func() bool { return false })
	assert.Equal(t, 1, ticked)
	assert.Zero(t, backpressure)
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
	send := func(_ context.Context, wallet int) (sentPacket, error) {
		if inFlight[wallet].Add(1) != 1 {
			t.Errorf("wallet %d has two transactions in flight", wallet)
		}
		defer inFlight[wallet].Add(-1)
		time.Sleep(2 * time.Millisecond)
		return sentPacket{key: packetKey{"client-0", sequence.Add(1)}, block: block.Load()}, nil
	}
	r := &DirectionReport{Route: "test", Target: target}
	out := make(chan sentPacket, target)
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

	generate(t.Context(), ticks, 3, wallets, send, out, r)

	var received []sentPacket
	for packet := range out {
		received = append(received, packet)
	}
	assert.Len(t, received, target)
	assert.Equal(t, target, r.Sent)
	assert.Zero(t, r.SendFailures)
	assert.Equal(t, target, r.SubmitToReceipt.Count)
	assert.GreaterOrEqual(t, r.Ticks, target/3)
	assert.Positive(t, r.BlocksWithTxs)
	assert.GreaterOrEqual(t, r.BlockSpan, r.BlocksWithTxs)
}

func TestGenerateRetiresFailingWallet(t *testing.T) {
	const wallets, target = 3, 12
	var calls [wallets]atomic.Int32
	var sequence atomic.Uint64
	send := func(_ context.Context, wallet int) (sentPacket, error) {
		calls[wallet].Add(1)
		if wallet == 0 {
			return sentPacket{}, errors.New("nonce too low")
		}
		return sentPacket{key: packetKey{"client-0", sequence.Add(1)}, block: 1}, nil
	}
	r := &DirectionReport{Route: "test", Target: target}
	out := make(chan sentPacket, target)

	generate(t.Context(), fakeTicks(t, time.Millisecond), wallets, wallets, send, out, r)

	assert.Equal(t, int32(1), calls[0].Load(), "a failed wallet sends no more")
	assert.Equal(t, 1, r.SendFailures)
	assert.Equal(t, target-1, r.Sent, "the failed send used up its slot")
	assert.Equal(t, int32(target-1), calls[1].Load()+calls[2].Load())
}

func TestGenerateEndsWhenEveryWalletRetired(t *testing.T) {
	send := func(context.Context, int) (sentPacket, error) { return sentPacket{}, errors.New("insufficient funds") }
	r := &DirectionReport{Route: "test", Target: 100}
	out := make(chan sentPacket, r.Target)

	finished := make(chan struct{})
	go func() {
		generate(t.Context(), fakeTicks(t, time.Millisecond), 5, 2, send, out, r)
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

func TestBlockCounts(t *testing.T) {
	blocks, span, low, median, high := blockCounts(map[uint64]int{10: 5, 11: 1, 14: 3})
	assert.Equal(t, []int{3, 5, 1, 3, 5}, []int{blocks, span, low, median, high})

	blocks, span, low, median, high = blockCounts(nil)
	assert.Equal(t, []int{0, 0, 0, 0, 0}, []int{blocks, span, low, median, high})
}

// The sender waits for its receipt; a canceled run must not leave senders behind.
func TestGenerateStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	var once sync.Once
	send := func(ctx context.Context, _ int) (sentPacket, error) {
		once.Do(cancel)
		<-ctx.Done()
		return sentPacket{}, ctx.Err()
	}
	r := &DirectionReport{Route: "test", Target: 10}
	generate(ctx, fakeTicks(t, time.Millisecond), 2, 2, send, make(chan sentPacket, 10), r)
	assert.Zero(t, r.Sent)
}
