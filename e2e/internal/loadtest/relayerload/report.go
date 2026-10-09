// SPDX-License-Identifier: Apache-2.0

package relayerload

import (
	"bytes"
	"encoding/json"
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
	Backpressure int `json:"backpressure"`
	Ticks        int `json:"ticks"`
	// SendPhaseMS runs from the first submission to the last receipt.
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

	lastSend  time.Time
	relayDone time.Time
}

// Report is the outcome of the relayer scenario.
type Report struct {
	Scenario string `json:"scenario"`
	Provider string `json:"provider"`
	Wallets  int    `json:"wallets"`
	PerBlock int    `json:"per_block"`
	Passed   bool   `json:"passed"`
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

func (r *Report) write(w io.Writer, asJSON bool) error {
	var out bytes.Buffer
	if asJSON {
		encoder := json.NewEncoder(&out)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(r); err != nil {
			return err
		}
	} else {
		r.text(&out)
	}
	_, err := w.Write(out.Bytes())
	return err
}

func (r *Report) text(out *bytes.Buffer) {
	verdict := "PASS"
	if !r.Passed {
		verdict = "FAIL"
	}
	wall := time.Duration(r.WallTimeMS) * time.Millisecond
	fmt.Fprintf(out, "\nRelayer load: %s (%s, %d wallets, %d per block interval): %d packets in %s, %.1f packets/s\n",
		verdict, r.Provider, r.Wallets, r.PerBlock, r.Packets, wall, r.Throughput)
	for _, d := range r.Directions {
		fmt.Fprintf(out, "\n[%s] %s -> %s\n", d.Route, d.Source, d.Destination)
		fmt.Fprintf(out, "  sends:    target=%d sent=%d send_failures=%d backpressure=%d ticks=%d send_phase=%s\n",
			d.Target, d.Sent, d.SendFailures, d.Backpressure, d.Ticks, ms(d.SendPhaseMS))
		fmt.Fprintf(out, "  blocks:   with_txs=%d span=%d per_block min=%d p50=%d max=%d\n",
			d.BlocksWithTxs, d.BlockSpan, d.PerBlockMin, d.PerBlockP50, d.PerBlockMax)
		fmt.Fprintf(out, "  packets:  succeeded=%d timed_out=%d rejected=%d relay_failed=%d unresolved=%d "+
			"relay_drain=%s\n", d.Succeeded, d.TimedOut, d.Rejected, d.RelayFailed, d.Unresolved, ms(d.RelayDrainMS))
		fmt.Fprintf(out, "  submit->receipt (wall clock): %s\n", d.SubmitToReceipt)
		fmt.Fprintf(out, "  send->recv  (block time):     %s\n", d.SendToRecv)
		fmt.Fprintf(out, "  recv->ack   (block time):     %s\n", d.RecvToAck)
		fmt.Fprintf(out, "  send->ack   (block time):     %s\n", d.SendToAck)
	}
	fmt.Fprintf(out, "\nRelay latencies come from block timestamps, so their precision is one block.\n")
	if r.Error != "" {
		fmt.Fprintf(out, "\nerrors:\n%s\n", r.Error)
	}
}

func ms(v int64) time.Duration { return time.Duration(v) * time.Millisecond }
