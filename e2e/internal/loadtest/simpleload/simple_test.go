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
	t.Run("sendsAtTheRate", func(t *testing.T) {
		var calls atomic.Int64
		spec := Spec{Duration: 500 * time.Millisecond, RatePerSecond: 100}

		result := SimpleLoad(t.Context(), spec, func(context.Context) error {
			calls.Add(1)
			return nil
		})

		assert.Equal(t, calls.Load(), result.Sent)
		assert.Equal(t, result.Sent, result.Succeeded)
		assert.EqualValues(t, 50, result.Sent, "100 rps for 0.5s")
		assert.InDelta(t, 500, result.DurationMS, 50)
		assert.InDelta(t, 100, result.AchievedRPS, 12)
		assert.Equal(t, int(result.Succeeded), result.Latency.Count)
	})

	t.Run("classifiesFailuresAndTimeouts", func(t *testing.T) {
		var calls atomic.Int64
		errs := []error{errors.New("boom"), context.DeadlineExceeded}
		spec := Spec{Duration: 300 * time.Millisecond, RatePerSecond: 100}

		result := SimpleLoad(t.Context(), spec, func(context.Context) error {
			return errs[int(calls.Add(1)-1)%len(errs)]
		})

		assert.Equal(t, result.Sent, result.Failed+result.TimedOut)
		assert.Positive(t, result.Failed)
		assert.Positive(t, result.TimedOut)
		assert.Zero(t, result.Succeeded)
		assert.Zero(t, result.Latency.Count, "latency covers succeeded calls only")
	})

	t.Run("keepsSendingWhileCallsAreSlow", func(t *testing.T) {
		spec := Spec{Duration: 200 * time.Millisecond, RatePerSecond: 100}
		started := time.Now()

		result := SimpleLoad(t.Context(), spec, func(context.Context) error {
			time.Sleep(400 * time.Millisecond)
			return nil
		})

		assert.EqualValues(t, 20, result.Sent, "open loop: slow calls do not hold back sends")
		assert.Equal(t, result.Sent, result.Succeeded)
		assert.GreaterOrEqual(t, time.Since(started), 400*time.Millisecond, "waits for calls in flight")
	})

	t.Run("sendsEveryCallOverAnOddDuration", func(t *testing.T) {
		spec := Spec{Duration: 290 * time.Millisecond, RatePerSecond: 100}

		result := SimpleLoad(t.Context(), spec, func(context.Context) error { return nil })

		assert.EqualValues(t, 29, result.Sent)
	})

	t.Run("sendsEveryCallAtRatesAboveTheTick", func(t *testing.T) {
		spec := Spec{Duration: 50 * time.Millisecond, RatePerSecond: 100_000}

		result := SimpleLoad(t.Context(), spec, func(context.Context) error { return nil })

		assert.EqualValues(t, 5000, result.Sent)
		assert.Equal(t, result.Sent, result.Succeeded)
	})

	t.Run("endsEarlyWhenTheRunEnds", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
		defer cancel()
		spec := Spec{Duration: time.Second, RatePerSecond: 100}

		result := SimpleLoad(ctx, spec, func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(150 * time.Millisecond):
				return nil
			}
		})

		assert.InDelta(t, 100, result.DurationMS, 50, "the duration sent, not the one asked for")
		assert.InDelta(t, 100, result.AchievedRPS, 30)
		assert.Positive(t, result.Sent)
		assert.Equal(t, result.Sent, result.Succeeded, "calls in flight are not cut short")
	})

	t.Run("encodesJSON", func(t *testing.T) {
		result := Result{Spec: Spec{RatePerSecond: 5}, Sent: 2, AchievedRPS: 2.5}
		encoded, err := json.Marshal(result)
		require.NoError(t, err)
		var decoded map[string]any
		require.NoError(t, json.Unmarshal(encoded, &decoded))
		assert.InDelta(t, 5, decoded["rate_per_second"], 0)
		assert.InDelta(t, 2.5, decoded["achieved_rps"], 0)
		assert.Contains(t, decoded, "latency")
	})
}
