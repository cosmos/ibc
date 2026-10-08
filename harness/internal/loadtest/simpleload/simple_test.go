// SPDX-License-Identifier: Apache-2.0

package simpleload

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunLoad(t *testing.T) {
	t.Run("succeeds", func(t *testing.T) {
		// ARRANGE
		var calls atomic.Int64
		spec := Spec{
			Duration:      time.Second,
			RatePerSecond: 50,
			Concurrency:   8,
		}
		call := func(context.Context) error {
			calls.Add(1)
			return nil
		}

		// ACT
		result := SimpleLoad(t.Context(), spec, call)

		// ASSERT
		require.Equal(t, calls.Load(), result.Succeeded)
		require.GreaterOrEqual(t, result.Succeeded, int64(50))
		assert.Zero(t, result.Failed)
		assert.Zero(t, result.TimedOut)
		assert.Zero(t, result.Dropped)
		assert.Len(t, result.Latencies, int(result.Succeeded))
		assert.Positive(t, result.Avg())
		assert.GreaterOrEqual(t, result.Percentile(95), result.Percentile(50))
	})

	t.Run("countsFailuresAndTimeouts", func(t *testing.T) {
		// ARRANGE
		var calls atomic.Int64
		errs := []error{errors.New("boom"), context.DeadlineExceeded}
		spec := Spec{
			Duration:      time.Second,
			RatePerSecond: 100,
			Concurrency:   len(errs),
		}
		call := func(context.Context) error {
			return errs[int(calls.Add(1)-1)%len(errs)]
		}

		// ACT
		result := SimpleLoad(t.Context(), spec, call)

		// ASSERT
		require.Equal(t, calls.Load(), result.Failed+result.TimedOut)
		assert.Zero(t, result.Succeeded)
		assert.Positive(t, result.Failed)
		assert.Positive(t, result.TimedOut)
		assert.Zero(t, result.Dropped)
	})

	t.Run("accountsForCallFinishingAfterRunEnds", func(t *testing.T) {
		// ARRANGE
		ctx, cancel := context.WithCancel(t.Context())
		t.Cleanup(cancel)

		started := make(chan struct{})
		spec := Spec{
			Duration:      time.Second,
			RatePerSecond: 1,
			Concurrency:   1,
		}
		call := func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			return nil
		}

		go func() {
			<-started
			cancel()
		}()

		// ACT
		result := SimpleLoad(ctx, spec, call)

		// ASSERT
		assert.Equal(t, int64(1), result.Succeeded)
		assert.Zero(t, result.Failed)
		assert.Zero(t, result.TimedOut)
		assert.Zero(t, result.Dropped)
		assert.Len(t, result.Latencies, 1)
	})

	t.Run("dropsWhenWorkersBusy", func(t *testing.T) {
		// ARRANGE
		spec := Spec{
			Duration:      time.Second,
			RatePerSecond: 10_000,
			Concurrency:   1,
		}
		call := func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Minute):
				return nil
			}
		}

		// ACT
		result := SimpleLoad(t.Context(), spec, call)

		// ASSERT
		assert.Zero(t, result.Succeeded)
		assert.Positive(t, result.Dropped)
	})
}

func TestResult(t *testing.T) {
	result := Result{Latencies: []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 60 * time.Millisecond}}

	assert.Equal(t, 30*time.Millisecond, result.Avg())
	assert.Equal(t, 20*time.Millisecond, result.Percentile(50))
	assert.Equal(t, 3, result.Summary().Latency.Count)
	assert.Zero(t, Result{}.Avg())
}
