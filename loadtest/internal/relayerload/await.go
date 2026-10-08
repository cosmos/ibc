// SPDX-License-Identifier: Apache-2.0

package relayerload

import (
	"context"
	"log/slog"
	"sync"
	"time"

	relayerv2 "github.com/cosmos/ibc/cli/api/v2/relayer"
	"github.com/cosmos/ibc/harness/environment"
	"github.com/cosmos/ibc/harness/ibccli"
	"github.com/cosmos/ibc/harness/traffic"
)

// trackedPacket is one packet located from an included source transaction.
type trackedPacket struct {
	packet     traffic.PacketTx
	includedAt time.Time
}

type awaitStats struct {
	Awaited     int
	Succeeded   int
	TimedOut    int
	Rejected    int
	RelayFailed int
	// Unresolved packets never reached a terminal state within the budget
	// (or failed status validation).
	Unresolved   int
	LastSettled  time.Time
	FirstFailure error
	// InclusionToSucceeded holds one sample per succeeded packet: from the
	// head that showed its source transaction to the status poll that first
	// saw it succeeded, so it is quantized by the poll interval.
	InclusionToSucceeded []time.Duration
}

// packetAwaitPolicy derives the per-packet wait from the route's own policy
// (itself derived from both Chains' environment.Timing):
//   - polls no faster than one block on either Chain, so many concurrent
//     waiters do not hammer the relayer status API with sub-block polls;
//   - budgets one route CompletionBudget per relayer batch the direction's
//     packets fill, because under sustained load a packet can queue behind
//     every batch ahead of it. Workers take packets in inclusion order, so a
//     packet's budget starts roughly when the packets before it settled.
func packetAwaitPolicy(
	route ibccli.WaitPolicy,
	source, destination environment.Timing,
	packets, batchSize int,
) ibccli.WaitPolicy {
	batches := max(1, (packets+batchSize-1)/max(batchSize, 1))
	return ibccli.WaitPolicy{
		CompletionBudget: route.CompletionBudget * time.Duration(batches),
		StatusPoll:       max(route.StatusPoll, source.BlockInterval, destination.BlockInterval),
		StabilityWindow:  route.StabilityWindow,
	}
}

// awaitPackets waits with workers bounded concurrency for every packet from
// packets to settle.
func awaitPackets(
	ctx context.Context,
	label string,
	relayer *ibccli.Relayer,
	policy ibccli.WaitPolicy,
	workers int,
	packets <-chan trackedPacket,
) awaitStats {
	var (
		mu    sync.Mutex
		stats awaitStats
		wg    sync.WaitGroup
	)
	progress := time.NewTicker(10 * time.Second)
	defer progress.Stop()
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-progress.C:
				mu.Lock()
				slog.InfoContext(ctx, "relay progress", "direction", label,
					"settled", stats.Awaited, "succeeded", stats.Succeeded)
				mu.Unlock()
			}
		}
	}()

	for range max(workers, 1) {
		wg.Go(func() {
			for tracked := range packets {
				state, _, err := traffic.AwaitTerminalWithPolicy(ctx, relayer, tracked.packet, policy)
				observedAt := time.Now()
				mu.Lock()
				stats.Awaited++
				if observedAt.After(stats.LastSettled) {
					stats.LastSettled = observedAt
				}
				switch {
				case err != nil:
					stats.Unresolved++
					if stats.FirstFailure == nil {
						stats.FirstFailure = err
					}
				case state == relayerv2.PacketState_PACKET_STATE_SUCCEEDED:
					stats.Succeeded++
					stats.InclusionToSucceeded = append(stats.InclusionToSucceeded, observedAt.Sub(tracked.includedAt))
				case state == relayerv2.PacketState_PACKET_STATE_TIMED_OUT:
					stats.TimedOut++
				case state == relayerv2.PacketState_PACKET_STATE_REJECTED:
					stats.Rejected++
				default:
					stats.RelayFailed++
				}
				mu.Unlock()
				if err == nil && state != relayerv2.PacketState_PACKET_STATE_SUCCEEDED {
					slog.WarnContext(ctx, "packet did not succeed", "direction", label,
						"packet", tracked.packet.String(), "tx", tracked.packet.SourceTxHash, "state", state)
				}
			}
		})
	}
	wg.Wait()
	close(stop)
	return stats
}
