// SPDX-License-Identifier: Apache-2.0

// Package stats summarizes latency samples.
package stats

import (
	"slices"
	"time"
)

// Percentile returns the nearest-rank-below p-th percentile of samples: the
// sample at index (n-1)*p/100 of the sorted samples. It returns zero for no
// samples or p outside [0, 100]. samples is not modified.
func Percentile(samples []time.Duration, p int) time.Duration {
	if len(samples) == 0 || p < 0 || p > 100 {
		return 0
	}
	sorted := slices.Sorted(slices.Values(samples))
	return sorted[(len(sorted)-1)*p/100]
}

// Mean returns the arithmetic mean of samples, or zero for none.
func Mean(samples []time.Duration) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	var total time.Duration
	for _, sample := range samples {
		total += sample
	}
	return total / time.Duration(len(samples))
}

// Latency is a JSON-friendly summary of latency samples in milliseconds.
type Latency struct {
	Count  int     `json:"count"`
	MeanMS float64 `json:"mean_ms"`
	P50MS  float64 `json:"p50_ms"`
	P90MS  float64 `json:"p90_ms"`
	P95MS  float64 `json:"p95_ms"`
	P99MS  float64 `json:"p99_ms"`
	MaxMS  float64 `json:"max_ms"`
}

// Summarize computes the Latency summary of samples.
func Summarize(samples []time.Duration) Latency {
	if len(samples) == 0 {
		return Latency{}
	}
	sorted := slices.Sorted(slices.Values(samples))
	at := func(p int) float64 { return ms(sorted[(len(sorted)-1)*p/100]) }
	return Latency{
		Count:  len(sorted),
		MeanMS: ms(Mean(sorted)),
		P50MS:  at(50),
		P90MS:  at(90),
		P95MS:  at(95),
		P99MS:  at(99),
		MaxMS:  ms(sorted[len(sorted)-1]),
	}
}

func ms(d time.Duration) float64 {
	return float64(d.Microseconds()) / 1000
}
