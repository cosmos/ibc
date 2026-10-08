// SPDX-License-Identifier: Apache-2.0

// Package stats summarizes latency samples.
package stats

import (
	"fmt"
	"slices"
	"time"
)

// Summary describes latency samples in milliseconds. Percentiles are
// nearest-rank-below: the sample at index (n-1)*p/100 of the sorted samples.
type Summary struct {
	Count int     `json:"count"`
	P50MS float64 `json:"p50_ms"`
	P90MS float64 `json:"p90_ms"`
	P95MS float64 `json:"p95_ms"`
	P99MS float64 `json:"p99_ms"`
	MaxMS float64 `json:"max_ms"`
}

// Summarize computes the Summary of samples without modifying them.
func Summarize(samples []time.Duration) Summary {
	if len(samples) == 0 {
		return Summary{}
	}
	sorted := slices.Sorted(slices.Values(samples))
	at := func(p int) float64 { return ms(sorted[(len(sorted)-1)*p/100]) }
	return Summary{
		Count: len(sorted),
		P50MS: at(50),
		P90MS: at(90),
		P95MS: at(95),
		P99MS: at(99),
		MaxMS: at(100),
	}
}

// String renders the summary on one line.
func (s Summary) String() string {
	if s.Count == 0 {
		return "n=0"
	}
	return fmt.Sprintf("n=%d p50=%s p90=%s p95=%s p99=%s max=%s",
		s.Count, d(s.P50MS), d(s.P90MS), d(s.P95MS), d(s.P99MS), d(s.MaxMS))
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

func d(ms float64) time.Duration {
	return time.Duration(ms * float64(time.Millisecond)).Round(time.Millisecond)
}
