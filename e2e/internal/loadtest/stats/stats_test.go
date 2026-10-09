// SPDX-License-Identifier: Apache-2.0

package stats

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSummarize(t *testing.T) {
	assert.Equal(t, Summary{}, Summarize(nil))
	assert.Equal(t, "n=0", Summarize(nil).String())

	// 100 samples of 1..100ms, shuffled order must not matter.
	samples := make([]time.Duration, 0, 100)
	for i := 100; i >= 1; i-- {
		samples = append(samples, time.Duration(i)*time.Millisecond)
	}
	got := Summarize(samples)
	assert.Equal(t, Summary{Count: 100, P50MS: 50, P90MS: 90, P95MS: 95, P99MS: 99, MaxMS: 100}, got)
	assert.Equal(t, 100*time.Millisecond, samples[0], "input is not modified")
	assert.Equal(t, "n=100 p50=50ms p90=90ms p95=95ms p99=99ms max=100ms", got.String())

	one := Summarize([]time.Duration{1500 * time.Microsecond})
	assert.Equal(t, Summary{Count: 1, P50MS: 1.5, P90MS: 1.5, P95MS: 1.5, P99MS: 1.5, MaxMS: 1.5}, one)
}
