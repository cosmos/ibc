// SPDX-License-Identifier: Apache-2.0

// Package simpleload sends calls at a fixed rate, open loop, and records
// their outcomes and latencies.
package simpleload

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/cosmos/ibc/e2e/internal/loadtest/stats"
)

// Timeout bounds a single call.
const Timeout = 3 * time.Second

// Spec shapes one load run. The CLI validates it: both fields positive.
type Spec struct {
	Duration      time.Duration `json:"-"`
	RatePerSecond int           `json:"rate_per_second"`
}

// Call is one request. Its context carries the per-call Timeout.
type Call func(ctx context.Context) error

// Result counts the calls a run sent and how they ended.
type Result struct {
	Spec
	// DurationMS is how long the run sent calls: Duration, or less if ctx
	// ended first.
	DurationMS int64 `json:"duration_ms"`
	Sent       int64 `json:"sent"`
	// AchievedRPS is the calls sent per second of DurationMS.
	AchievedRPS float64 `json:"achieved_rps"`
	Succeeded   int64   `json:"succeeded"`
	Failed      int64   `json:"failed"`
	TimedOut    int64   `json:"timed_out"`
	// Latency summarizes the succeeded calls.
	Latency    stats.Summary `json:"latency"`
	FirstError string        `json:"first_error,omitempty"`
}

// SimpleLoad sends spec.RatePerSecond calls per second for spec.Duration,
// each on its own goroutine however many are in flight, so a slow service
// shows as latency and timeouts rather than as fewer calls. It stops sending
// early when ctx ends. A call's Timeout context is detached from ctx's
// cancellation: calls in flight when sending stops run to completion, and
// SimpleLoad waits for them.
func SimpleLoad(ctx context.Context, spec Spec, call Call) Result {
	ticker := time.NewTicker(time.Second / time.Duration(spec.RatePerSecond))
	defer ticker.Stop()
	phase := time.NewTimer(spec.Duration)
	defer phase.Stop()
	detached := context.WithoutCancel(ctx)

	result := Result{Spec: spec}
	var (
		mu        sync.Mutex
		latencies []time.Duration
		wg        sync.WaitGroup
	)
	send := func() {
		callCtx, cancel := context.WithTimeout(detached, Timeout)
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
	}

	started := time.Now()
	for sending := true; sending; {
		select {
		case <-ctx.Done():
			sending = false
		case <-phase.C:
			sending = false
		case <-ticker.C:
			result.Sent++
			wg.Go(send)
		}
	}
	elapsed := time.Since(started)
	wg.Wait()
	result.DurationMS = elapsed.Milliseconds()
	result.AchievedRPS = float64(result.Sent) / elapsed.Seconds()
	result.Latency = stats.Summarize(latencies)
	return result
}
