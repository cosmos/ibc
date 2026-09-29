// SPDX-License-Identifier: Apache-2.0

package loadtest

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
	t.Run("avg", func(t *testing.T) {
		for _, tt := range []struct {
			name      string
			latencies []time.Duration
			expected  time.Duration
		}{
			{
				name:      "empty",
				latencies: nil,
				expected:  0,
			},
			{
				name:      "single",
				latencies: []time.Duration{42 * time.Millisecond},
				expected:  42 * time.Millisecond,
			},
			{
				name:      "multiple",
				latencies: []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 60 * time.Millisecond},
				expected:  30 * time.Millisecond,
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				result := Result{Latencies: tt.latencies}

				assert.Equal(t, tt.expected, result.Avg())
			})
		}
	})

	t.Run("percentile", func(t *testing.T) {
		latencies := make([]time.Duration, 0, 10)
		for i := 1; i <= 10; i++ {
			latencies = append(latencies, time.Duration(i*10)*time.Millisecond)
		}

		for _, tt := range []struct {
			name      string
			latencies []time.Duration
			p         int
			expected  time.Duration
		}{
			{
				name:      "empty",
				latencies: nil,
				p:         50,
				expected:  0,
			},
			{
				name:      "outOfRange",
				latencies: latencies,
				p:         101,
				expected:  0,
			},
			{
				name:      "p0",
				latencies: latencies,
				p:         0,
				expected:  10 * time.Millisecond,
			},
			{
				name:      "p50",
				latencies: latencies,
				p:         50,
				expected:  50 * time.Millisecond,
			},
			{
				name:      "p90",
				latencies: latencies,
				p:         90,
				expected:  90 * time.Millisecond,
			},
			{
				name:      "p100",
				latencies: latencies,
				p:         100,
				expected:  100 * time.Millisecond,
			},
			{
				name:      "unsortedInput",
				latencies: []time.Duration{90 * time.Millisecond, 10 * time.Millisecond, 50 * time.Millisecond},
				p:         50,
				expected:  50 * time.Millisecond,
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				result := Result{Latencies: tt.latencies}

				assert.Equal(t, tt.expected, result.Percentile(tt.p))
			})
		}
	})
}
