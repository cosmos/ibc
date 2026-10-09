// SPDX-License-Identifier: Apache-2.0

// Package simpleload offers calls at a fixed rate to a bounded number of
// concurrent callers and records their outcomes and latencies.
package simpleload

import (
	"context"
	"errors"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/cosmos/ibc/e2e/internal/loadtest/stats"
)

// Timeout bounds a single call.
const Timeout = 3 * time.Second

// Spec shapes one load run. The CLI validates it: every field positive.
type Spec struct {
	Duration      time.Duration `json:"-"`
	RatePerSecond int           `json:"rate_per_second"`
	Concurrency   int           `json:"concurrency"`
}

// Call is one request. Its context carries the per-call Timeout.
type Call func(ctx context.Context) error

// Result counts what a run offered and how the calls ended.
type Result struct {
	Spec
	DurationMS int64 `json:"duration_ms"`
	// Offered is Sent + Dropped: every slot the rate limiter opened.
	Offered int64 `json:"offered"`
	Sent    int64 `json:"sent"`
	// Dropped slots found every caller busy.
	Dropped int64 `json:"dropped"`
	// AchievedRPS is the calls sent per second of the phase: the rate that
	// reached the service, at most RatePerSecond.
	AchievedRPS float64 `json:"achieved_rps"`
	Succeeded   int64   `json:"succeeded"`
	Failed      int64   `json:"failed"`
	TimedOut    int64   `json:"timed_out"`
	// Latency summarizes the succeeded calls.
	Latency    stats.Summary `json:"latency"`
	FirstError string        `json:"first_error,omitempty"`
}

// SimpleLoad offers spec.RatePerSecond calls per second for spec.Duration.
// An offered call starts only if fewer than spec.Concurrency calls are in
// flight; otherwise it is dropped. Each call's context derives from ctx, not
// from the phase deadline, so calls in flight at the deadline run to
// completion (bounded by Timeout) and SimpleLoad waits for them.
func SimpleLoad(ctx context.Context, spec Spec, call Call) Result {
	// The phase has its own deadline and ends early only when ctx ends: the
	// limiter fails a Wait whose slot would fall past a deadline, so
	// inheriting an earlier ctx deadline would end the phase before ctx
	// reports it, as if the phase had run its full duration.
	phase, cancel := context.WithTimeout(context.WithoutCancel(ctx), spec.Duration)
	defer cancel()
	defer context.AfterFunc(ctx, cancel)()
	limiter := rate.NewLimiter(rate.Limit(spec.RatePerSecond), 1)
	callers := make(chan struct{}, spec.Concurrency)

	result := Result{Spec: spec, DurationMS: spec.Duration.Milliseconds()}
	var (
		mu        sync.Mutex
		latencies []time.Duration
		wg        sync.WaitGroup
	)
	// Wait fails once the next slot would fall past the phase deadline.
	for limiter.Wait(phase) == nil {
		select {
		case callers <- struct{}{}:
		default:
			result.Dropped++
			continue
		}
		result.Sent++
		wg.Go(func() {
			defer func() { <-callers }()
			callCtx, cancel := context.WithTimeout(ctx, Timeout)
			defer cancel()
			started := time.Now()
			err := call(callCtx)
			latency := time.Since(started)

			mu.Lock()
			defer mu.Unlock()
			if err != nil && result.FirstError == "" {
				result.FirstError = err.Error()
			}
			switch {
			case err == nil:
				result.Succeeded++
				latencies = append(latencies, latency)
			case errors.Is(err, context.DeadlineExceeded) || errors.Is(callCtx.Err(), context.DeadlineExceeded):
				result.TimedOut++
			default:
				result.Failed++
			}
		})
	}
	wg.Wait()
	result.Offered = result.Sent + result.Dropped
	result.AchievedRPS = float64(result.Sent) / spec.Duration.Seconds()
	result.Latency = stats.Summarize(latencies)
	return result
}
