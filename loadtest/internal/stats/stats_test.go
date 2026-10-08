// SPDX-License-Identifier: Apache-2.0

package stats

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPercentile(t *testing.T) {
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
		{name: "empty", latencies: nil, p: 50, expected: 0},
		{name: "outOfRange", latencies: latencies, p: 101, expected: 0},
		{name: "negative", latencies: latencies, p: -1, expected: 0},
		{name: "p0", latencies: latencies, p: 0, expected: 10 * time.Millisecond},
		{name: "p50", latencies: latencies, p: 50, expected: 50 * time.Millisecond},
		{name: "p90", latencies: latencies, p: 90, expected: 90 * time.Millisecond},
		{name: "p100", latencies: latencies, p: 100, expected: 100 * time.Millisecond},
		{
			name:      "unsortedInput",
			latencies: []time.Duration{90 * time.Millisecond, 10 * time.Millisecond, 50 * time.Millisecond},
			p:         50,
			expected:  50 * time.Millisecond,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, Percentile(tt.latencies, tt.p))
		})
	}
}

func TestPercentileDoesNotReorderInput(t *testing.T) {
	samples := []time.Duration{3, 1, 2}
	Percentile(samples, 50)
	assert.Equal(t, []time.Duration{3, 1, 2}, samples)
}

func TestMean(t *testing.T) {
	assert.Zero(t, Mean(nil))
	assert.Equal(t, 42*time.Millisecond, Mean([]time.Duration{42 * time.Millisecond}))
	assert.Equal(t, 30*time.Millisecond, Mean([]time.Duration{
		10 * time.Millisecond, 20 * time.Millisecond, 60 * time.Millisecond,
	}))
}

func TestSummarize(t *testing.T) {
	assert.Equal(t, Latency{}, Summarize(nil))

	samples := make([]time.Duration, 0, 100)
	for i := 100; i >= 1; i-- {
		samples = append(samples, time.Duration(i)*time.Millisecond)
	}
	assert.Equal(t, Latency{
		Count:  100,
		MeanMS: 50.5,
		P50MS:  50,
		P90MS:  90,
		P95MS:  95,
		P99MS:  99,
		MaxMS:  100,
	}, Summarize(samples))
}
