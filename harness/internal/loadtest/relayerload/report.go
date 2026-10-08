// SPDX-License-Identifier: Apache-2.0

package relayerload

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/cosmos/ibc/harness/internal/loadtest/stats"
	"github.com/cosmos/ibc/harness/internal/loadtest/textout"
)

// DirectionReport is the outcome of one direction of the relayer scenario.
type DirectionReport struct {
	Route       string `json:"route"`
	Source      string `json:"source"`
	Destination string `json:"destination"`

	// Transactions.
	Target       int `json:"target"`
	Attempted    int `json:"attempted"`
	Sent         int `json:"sent"`
	SendErrors   int `json:"send_errors"`
	Included     int `json:"included"`
	Reverted     int `json:"reverted"`
	Dropped      int `json:"dropped"`
	Backpressure int `json:"backpressure"`
	HeadsSeen    int `json:"heads_seen"`
	Dispatches   int `json:"dispatches"`

	// Packets.
	Packets            int `json:"packets"`
	MissingPackets     int `json:"missing_packets"`
	PacketsSucceeded   int `json:"packets_succeeded"`
	PacketsTimedOut    int `json:"packets_timed_out"`
	PacketsRejected    int `json:"packets_rejected"`
	PacketsRelayFailed int `json:"packets_relay_failed"`
	PacketsUnresolved  int `json:"packets_unresolved"`

	SendPhaseMS  int64 `json:"send_phase_ms"`
	RelayDrainMS int64 `json:"relay_drain_ms"`

	SendToInclusion      stats.Latency `json:"send_to_inclusion"`
	InclusionToSucceeded stats.Latency `json:"inclusion_to_succeeded"`

	Error string `json:"error,omitempty"`
}

// Failed reports whether the direction missed its target.
func (d DirectionReport) Failed() bool {
	return d.Error != "" || d.Attempted != d.Target || d.SendErrors > 0 || d.Reverted > 0 || d.Dropped > 0 ||
		d.MissingPackets > 0 || d.PacketsSucceeded != d.Sent
}

// Report is the outcome of the relayer scenario.
type Report struct {
	Scenario   string            `json:"scenario"`
	Provider   string            `json:"provider"`
	Wallets    int               `json:"wallets"`
	PerBlock   int               `json:"per_block"`
	Directions []DirectionReport `json:"directions"`
	Total      DirectionReport   `json:"total"`
	DurationMS int64             `json:"duration_ms"`
	Passed     bool              `json:"passed"`
}

func newDirectionReport(
	label, source, destination string,
	target int,
	gen generatorStats,
	await awaitStats,
	packets, missing int,
	err error,
) DirectionReport {
	report := DirectionReport{
		Route:                label,
		Source:               source,
		Destination:          destination,
		Target:               target,
		Attempted:            gen.Attempted,
		Sent:                 gen.Sent,
		SendErrors:           gen.SendErrors,
		Included:             gen.Included,
		Reverted:             gen.Reverted,
		Dropped:              gen.Dropped,
		Backpressure:         gen.Backpressure,
		HeadsSeen:            gen.HeadsSeen,
		Dispatches:           gen.Dispatches,
		Packets:              packets,
		MissingPackets:       missing,
		PacketsSucceeded:     await.Succeeded,
		PacketsTimedOut:      await.TimedOut,
		PacketsRejected:      await.Rejected,
		PacketsRelayFailed:   await.RelayFailed,
		PacketsUnresolved:    await.Unresolved,
		SendToInclusion:      stats.Summarize(gen.SendToInclusion),
		InclusionToSucceeded: stats.Summarize(await.InclusionToSucceeded),
	}
	if !gen.FirstSend.IsZero() {
		report.SendPhaseMS = gen.LastSend.Sub(gen.FirstSend).Milliseconds()
		if await.LastSettled.After(gen.LastSend) {
			report.RelayDrainMS = await.LastSettled.Sub(gen.LastSend).Milliseconds()
		}
	}
	errs := []string{}
	if err != nil {
		errs = append(errs, err.Error())
	}
	if await.FirstFailure != nil {
		errs = append(errs, "first unresolved packet: "+await.FirstFailure.Error())
	}
	report.Error = strings.Join(errs, "; ")
	return report
}

// total sums direction counters. Latency summaries are recomputed from the
// raw samples by the caller.
func total(directions []DirectionReport) DirectionReport {
	var t DirectionReport
	t.Route = "total"
	for _, d := range directions {
		t.Target += d.Target
		t.Attempted += d.Attempted
		t.Sent += d.Sent
		t.SendErrors += d.SendErrors
		t.Included += d.Included
		t.Reverted += d.Reverted
		t.Dropped += d.Dropped
		t.Backpressure += d.Backpressure
		t.HeadsSeen += d.HeadsSeen
		t.Dispatches += d.Dispatches
		t.Packets += d.Packets
		t.MissingPackets += d.MissingPackets
		t.PacketsSucceeded += d.PacketsSucceeded
		t.PacketsTimedOut += d.PacketsTimedOut
		t.PacketsRejected += d.PacketsRejected
		t.PacketsRelayFailed += d.PacketsRelayFailed
		t.PacketsUnresolved += d.PacketsUnresolved
		t.SendPhaseMS = max(t.SendPhaseMS, d.SendPhaseMS)
		t.RelayDrainMS = max(t.RelayDrainMS, d.RelayDrainMS)
		if d.Error != "" {
			t.Error = strings.TrimPrefix(t.Error+"; "+d.Route+": "+d.Error, "; ")
		}
	}
	return t
}

// WriteText renders the report for humans.
func (r *Report) WriteText(w io.Writer) error {
	out := textout.New(w)
	verdict := "PASS"
	if !r.Passed {
		verdict = "FAIL"
	}
	out.Printf("\nRelayer load: %s (%s, %d wallets, %d per block) in %s\n",
		verdict, r.Provider, r.Wallets, r.PerBlock, msDuration(r.DurationMS))
	for _, d := range append(append([]DirectionReport{}, r.Directions...), r.Total) {
		if d.Source == "" {
			out.Printf("\n[%s]\n", d.Route)
		} else {
			out.Printf("\n[%s] %s -> %s\n", d.Route, d.Source, d.Destination)
		}
		out.Printf("  transactions: target=%d attempted=%d sent=%d send_errors=%d included=%d reverted=%d dropped=%d\n",
			d.Target, d.Attempted, d.Sent, d.SendErrors, d.Included, d.Reverted, d.Dropped)
		out.Printf("  pacing:       heads=%d dispatches=%d backpressure=%d\n",
			d.HeadsSeen, d.Dispatches, d.Backpressure)
		out.Printf("  packets:      located=%d missing=%d succeeded=%d timed_out=%d rejected=%d "+
			"relay_failed=%d unresolved=%d\n",
			d.Packets, d.MissingPackets, d.PacketsSucceeded, d.PacketsTimedOut, d.PacketsRejected,
			d.PacketsRelayFailed, d.PacketsUnresolved)
		out.Printf("  durations:    send_phase=%s relay_drain=%s\n",
			msDuration(d.SendPhaseMS), msDuration(d.RelayDrainMS))
		out.Printf("  send->inclusion:        %s\n", latencyLine(d.SendToInclusion))
		out.Printf("  inclusion->succeeded*:  %s\n", latencyLine(d.InclusionToSucceeded))
		if d.Error != "" {
			out.Printf("  error: %s\n", d.Error)
		}
	}
	out.Printf("\n* observed by polling the relayer status API, so quantized by the poll interval;\n")
	out.Printf("  precise relay lifecycle timestamps are tracked in FOU-1740.\n")
	return out.Err()
}

func latencyLine(l stats.Latency) string {
	if l.Count == 0 {
		return "n=0"
	}
	return fmt.Sprintf("n=%d mean=%s p50=%s p90=%s p95=%s p99=%s max=%s", l.Count,
		fmsDuration(l.MeanMS), fmsDuration(l.P50MS), fmsDuration(l.P90MS),
		fmsDuration(l.P95MS), fmsDuration(l.P99MS), fmsDuration(l.MaxMS))
}

func msDuration(ms int64) time.Duration { return time.Duration(ms) * time.Millisecond }

func fmsDuration(ms float64) time.Duration {
	return time.Duration(ms * float64(time.Millisecond)).Round(time.Millisecond)
}
