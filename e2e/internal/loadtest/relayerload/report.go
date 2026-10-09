// SPDX-License-Identifier: Apache-2.0

package relayerload

import (
	"bytes"
	"fmt"
	"io"
	"time"

	"github.com/cosmos/ibc/e2e/internal/loadtest/stats"
)

// DirectionReport is the outcome of one direction. The senders, the relay
// sweep, and the latency measurement fill it in directly.
type DirectionReport struct {
	Route       string `json:"route"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Target      int    `json:"target"`

	// Sending.
	Sent         int `json:"sent"`
	SendFailures int `json:"send_failures"`
	// FirstSendError is the error that retired the first wallet.
	FirstSendError string `json:"first_send_error,omitempty"`
	Backpressure   int    `json:"backpressure"`
	// SendPhaseMS runs from the first offered send to the last receipt.
	SendPhaseMS int64 `json:"send_phase_ms"`
	// SubmitToReceipt is wall clock from submitting a transfer to holding
	// its receipt.
	SubmitToReceipt stats.Summary `json:"submit_to_receipt"`

	// Achieved pacing, from the blocks that included the transfers.
	BlocksWithTxs int `json:"blocks_with_txs"`
	BlockSpan     int `json:"block_span"`
	PerBlockMin   int `json:"per_block_min"`
	PerBlockP50   int `json:"per_block_p50"`
	PerBlockMax   int `json:"per_block_max"`

	// Relaying.
	Succeeded   int `json:"succeeded"`
	TimedOut    int `json:"timed_out"`
	Rejected    int `json:"rejected"`
	RelayFailed int `json:"relay_failed"`
	Unresolved  int `json:"unresolved"`
	// RelayDrainMS runs from the last sent packet to the end of the sweep.
	RelayDrainMS int64 `json:"relay_drain_ms"`
	// Relay latencies of succeeded packets, from block timestamps.
	SendToRecv stats.Summary `json:"send_to_recv"`
	RecvToAck  stats.Summary `json:"recv_to_ack"`
	SendToAck  stats.Summary `json:"send_to_ack"`
	// LatencyError is the first failed chain read of the latency
	// measurement; the packets that needed it have no samples.
	LatencyError string `json:"latency_error,omitempty"`

	lastSend  time.Time
	relayDone time.Time
}

// Report is the outcome of the relayer scenario.
type Report struct {
	Scenario string `json:"scenario"`
	Provider string `json:"provider"`
	Wallets  int    `json:"wallets"`
	PerBlock int    `json:"per_block"`
	// Passed reports that the relayer relayed every packet sent to
	// SUCCEEDED, without a stall or an interruption.
	Passed bool `json:"passed"`
	// LoadDelivered reports that every direction also sent its target; a
	// run that Passed without it is inconclusive.
	LoadDelivered bool `json:"load_delivered"`
	// Packets is the target across directions.
	Packets int `json:"packets"`
	// WallTimeMS runs from the start of sending to the end of the last
	// direction's relay sweep.
	WallTimeMS int64 `json:"wall_time_ms"`
	// Throughput is succeeded packets per second of wall time.
	Throughput float64            `json:"throughput_packets_per_second"`
	Error      string             `json:"error,omitempty"`
	Directions []*DirectionReport `json:"directions"`
}

// WriteText writes the report for people.
func (r *Report) WriteText(w io.Writer) error {
	var out bytes.Buffer
	verdict := "PASS"
	switch {
	case !r.Passed:
		verdict = "FAIL"
	case !r.LoadDelivered:
		verdict = "INCONCLUSIVE (load not delivered)"
	}
	wall := time.Duration(r.WallTimeMS) * time.Millisecond
	fmt.Fprintf(&out, "\nRelayer load: %s (%s, %d wallets, %d per block interval): %d packets in %s, %.1f packets/s\n",
		verdict, r.Provider, r.Wallets, r.PerBlock, r.Packets, wall, r.Throughput)
	for _, d := range r.Directions {
		fmt.Fprintf(&out, "\n[%s] %s -> %s\n", d.Route, d.Source, d.Destination)
		fmt.Fprintf(&out, "  sends:    target=%d sent=%d send_failures=%d backpressure=%d send_phase=%s\n",
			d.Target, d.Sent, d.SendFailures, d.Backpressure, ms(d.SendPhaseMS))
		fmt.Fprintf(&out, "  blocks:   with_txs=%d span=%d per_block min=%d p50=%d max=%d\n",
			d.BlocksWithTxs, d.BlockSpan, d.PerBlockMin, d.PerBlockP50, d.PerBlockMax)
		fmt.Fprintf(&out, "  packets:  succeeded=%d timed_out=%d rejected=%d relay_failed=%d unresolved=%d "+
			"relay_drain=%s\n", d.Succeeded, d.TimedOut, d.Rejected, d.RelayFailed, d.Unresolved, ms(d.RelayDrainMS))
		fmt.Fprintf(&out, "  submit->receipt (wall clock): %s\n", d.SubmitToReceipt)
		fmt.Fprintf(&out, "  send->recv  (block time):     %s\n", d.SendToRecv)
		fmt.Fprintf(&out, "  recv->ack   (block time):     %s\n", d.RecvToAck)
		fmt.Fprintf(&out, "  send->ack   (block time):     %s\n", d.SendToAck)
		if d.LatencyError != "" {
			fmt.Fprintf(&out, "  latency error: %s\n", d.LatencyError)
		}
	}
	fmt.Fprintf(&out, "\nRelay latencies come from block timestamps, so their precision is one block.\n")
	if r.Error != "" {
		fmt.Fprintf(&out, "\nerrors:\n%s\n", r.Error)
	}
	_, err := w.Write(out.Bytes())
	return err
}

func ms(v int64) time.Duration { return time.Duration(v) * time.Millisecond }
