package loadtest

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"
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

		// The run ended while this call was in flight.
		if ctx.Err() != nil {
			return
		}

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

func (r Result) Avg() time.Duration {
	if len(r.Latencies) == 0 {
		return 0
	}

	var total time.Duration
	for _, latency := range r.Latencies {
		total += latency
	}

	return total / time.Duration(len(r.Latencies))
}

func (r Result) Percentile(p int) time.Duration {
	if len(r.Latencies) == 0 {
		return 0
	}
	if p < 0 || p > 100 {
		return 0
	}
	sorted := slices.Sorted(slices.Values(r.Latencies))
	return sorted[(len(sorted)-1)*p/100]
}

func (r Result) LogT(t testing.TB) {
	t.Helper()

	type kv struct {
		key   string
		value any
	}

	kvs := []kv{
		{key: "spec.duration", value: r.spec.Duration.String()},
		{key: "spec.rps", value: r.spec.RatePerSecond},
		{key: "spec.concurrency", value: r.spec.Concurrency},

		{key: "reqs.succeeded", value: r.Succeeded},
		{key: "reqs.failed", value: r.Failed},
		{key: "reqs.timed_out", value: r.TimedOut},
		{key: "reqs.dropped", value: r.Dropped},

		{key: "latency.avg", value: r.Avg()},
		{key: "latency.p90", value: r.Percentile(90)},
		{key: "latency.p95", value: r.Percentile(95)},
		{key: "latency.p99", value: r.Percentile(99)},
	}

	t.Logf("Load test results:")
	for _, kv := range kvs {
		t.Logf("  %s: %v", kv.key, kv.value)
	}
}
