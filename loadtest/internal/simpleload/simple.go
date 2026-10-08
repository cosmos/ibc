// SPDX-License-Identifier: Apache-2.0

// Package simpleload offers calls at a fixed rate from a bounded worker pool
// and records their outcomes and latencies.
package simpleload

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cosmos/ibc/loadtest/internal/stats"
)

// Spec shapes one load run.
type Spec struct {
	// Duration test duration
	Duration time.Duration
	// RatePerSecond How many RPS should we submit?
	RatePerSecond int
	// Concurrency How many concurrent requests should we submit?
	Concurrency int
}

// Timeout bounds a single call.
const Timeout = 3 * time.Second

type Call func(ctx context.Context) error

type Result struct {
	Succeeded int64
	Failed    int64
	TimedOut  int64
	Dropped   int64

	Latencies []time.Duration

	spec Spec
}

// SimpleLoad runs N times per second with C concurrent callers for D duration.
func SimpleLoad(ctx context.Context, spec Spec, call Call) Result {
	spec.Concurrency = max(spec.Concurrency, 1)
	spec.RatePerSecond = max(spec.RatePerSecond, 1)
	spec.Duration = max(spec.Duration, time.Second)

	ctx, cancel := context.WithTimeout(ctx, spec.Duration)
	defer cancel()

	var (
		succeeded, failed, timedOut, dropped, iterations atomic.Int64
		mu                                               sync.Mutex
		latencies                                        []time.Duration
	)

	wrappedCall := func() {
		callCtx, cancel := context.WithTimeout(ctx, Timeout)
		defer cancel()

		start := time.Now()
		err := call(callCtx)
		latency := time.Since(start)

		switch {
		case errors.Is(err, context.DeadlineExceeded):
			timedOut.Add(1)
		case err != nil:
			failed.Add(1)
			mu.Lock()
			latencies = append(latencies, latency)
			mu.Unlock()
		default:
			succeeded.Add(1)
			mu.Lock()
			latencies = append(latencies, latency)
			mu.Unlock()
		}
	}

	// spawn N concurrent requesters
	jobs := make(chan struct{}, spec.Concurrency)
	var wg sync.WaitGroup
	for range spec.Concurrency {
		wg.Go(func() {
			// loop until jobs is closed
			for range jobs {
				wrappedCall()

				if i := iterations.Add(1); i%1000 == 0 {
					slog.InfoContext(ctx, "SimpleLoad", "iterations", i)
				}
			}
		})
	}

	// Offer one call per interval. A whole second's batch at once would
	// overflow the queue instantly and drop calls the workers have time to
	// process; spreading them means drops happen only when workers fall
	// behind the offered rate.
	interval := time.Second / time.Duration(spec.RatePerSecond)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	tick := func() {
		select {
		case jobs <- struct{}{}:
		default:
			dropped.Add(1)
		}
	}

	// first tick
	tick()

	for {
		select {
		case <-ticker.C:
			tick()
		case <-ctx.Done():
			close(jobs)
			wg.Wait()

			return Result{
				Succeeded: succeeded.Load(),
				Failed:    failed.Load(),
				TimedOut:  timedOut.Load(),
				Dropped:   dropped.Load(),
				Latencies: latencies,

				spec: spec,
			}
		}
	}
}

// Spec returns the normalized spec the run used.
func (r Result) Spec() Spec { return r.spec }

func (r Result) Avg() time.Duration { return stats.Mean(r.Latencies) }

func (r Result) Percentile(p int) time.Duration { return stats.Percentile(r.Latencies, p) }

// Summary is the JSON-friendly view of a Result.
type Summary struct {
	DurationMS    int64         `json:"duration_ms"`
	RatePerSecond int           `json:"rate_per_second"`
	Concurrency   int           `json:"concurrency"`
	Succeeded     int64         `json:"succeeded"`
	Failed        int64         `json:"failed"`
	TimedOut      int64         `json:"timed_out"`
	Dropped       int64         `json:"dropped"`
	Latency       stats.Latency `json:"latency"`
}

// Summary summarizes the result for reporting.
func (r Result) Summary() Summary {
	return Summary{
		DurationMS:    r.spec.Duration.Milliseconds(),
		RatePerSecond: r.spec.RatePerSecond,
		Concurrency:   r.spec.Concurrency,
		Succeeded:     r.Succeeded,
		Failed:        r.Failed,
		TimedOut:      r.TimedOut,
		Dropped:       r.Dropped,
		Latency:       stats.Summarize(r.Latencies),
	}
}
