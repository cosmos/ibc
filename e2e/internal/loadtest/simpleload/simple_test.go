// SPDX-License-Identifier: Apache-2.0

package simpleload

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSimpleLoad(t *testing.T) {
	t.Run("offersAtTheRate", func(t *testing.T) {
		var calls atomic.Int64
		spec := Spec{Duration: 500 * time.Millisecond, RatePerSecond: 100, Concurrency: 8}

		result := SimpleLoad(t.Context(), spec, func(context.Context) error {
			calls.Add(1)
			return nil
		})

		assert.Equal(t, calls.Load(), result.Sent)
		assert.Equal(t, result.Sent, result.Succeeded)
		assert.Equal(t, result.Sent+result.Dropped, result.Offered)
		assert.InDelta(t, 50, result.Offered, 6, "100 rps for 0.5s")
		assert.InDelta(t, float64(result.Sent)*2, result.AchievedRPS, 0.001)
		assert.Zero(t, result.Dropped)
		assert.Equal(t, int(result.Succeeded), result.Latency.Count)
	})

	t.Run("classifiesFailuresAndTimeouts", func(t *testing.T) {
		var calls atomic.Int64
		errs := []error{errors.New("boom"), context.DeadlineExceeded}
		spec := Spec{Duration: 300 * time.Millisecond, RatePerSecond: 100, Concurrency: 4}

		result := SimpleLoad(t.Context(), spec, func(context.Context) error {
			return errs[int(calls.Add(1)-1)%len(errs)]
		})

		assert.Equal(t, result.Sent, result.Failed+result.TimedOut)
		assert.Positive(t, result.Failed)
		assert.Positive(t, result.TimedOut)
		assert.Zero(t, result.Succeeded)
		assert.Zero(t, result.Latency.Count, "latency covers succeeded calls only")
	})

	t.Run("dropsOnlyWhenEveryCallerIsBusy", func(t *testing.T) {
		spec := Spec{Duration: 200 * time.Millisecond, RatePerSecond: 500, Concurrency: 2}

		result := SimpleLoad(t.Context(), spec, func(context.Context) error {
			time.Sleep(400 * time.Millisecond)
			return nil
		})

		assert.Equal(t, int64(2), result.Sent, "two callers, each busy past the phase")
		assert.Equal(t, int64(2), result.Succeeded)
		assert.Equal(t, result.Offered-2, result.Dropped)
		assert.Greater(t, result.Offered, int64(50))
	})

	t.Run("callsInFlightOutliveThePhaseDeadline", func(t *testing.T) {
		spec := Spec{Duration: 50 * time.Millisecond, RatePerSecond: 1, Concurrency: 1}
		started := time.Now()

		result := SimpleLoad(t.Context(), spec, func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(200 * time.Millisecond):
				return nil
			}
		})

		assert.Equal(t, int64(1), result.Succeeded, "the call's context is not the phase's")
		assert.GreaterOrEqual(t, time.Since(started), 200*time.Millisecond, "waits for calls in flight")
	})

	t.Run("endsWithAnEarlierRunDeadline", func(t *testing.T) {
		// The next slot (200ms) falls past the run deadline (100ms): the phase
		// must last until the run reports its end, not stop at once.
		ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
		defer cancel()
		spec := Spec{Duration: time.Second, RatePerSecond: 5, Concurrency: 1}

		SimpleLoad(ctx, spec, func(context.Context) error { return nil })

		require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded, "ended by the run, which reports it")
	})

	t.Run("encodesJSON", func(t *testing.T) {
		result := Result{Spec: Spec{RatePerSecond: 5, Concurrency: 2}, Offered: 3, Sent: 2, Dropped: 1}
		encoded, err := json.Marshal(result)
		require.NoError(t, err)
		var decoded map[string]any
		require.NoError(t, json.Unmarshal(encoded, &decoded))
		assert.InDelta(t, 5, decoded["rate_per_second"], 0)
		assert.InDelta(t, 3, decoded["offered"], 0)
		assert.Contains(t, decoded, "latency")
	})
}
