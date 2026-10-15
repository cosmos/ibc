// SPDX-License-Identifier: Apache-2.0

package loadtest

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	catalystevm "github.com/skip-mev/catalyst/chains/ethereum/types"

	relayerv2 "github.com/cosmos/ibc/cli/api/v2/relayer"
	"github.com/cosmos/ibc/e2e/internal/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/ibccli"
)

const (
	intervalPacketPoll = 2 * time.Second
	maxLookupAttempts  = 10
	maxLookupsPerPoll  = 200
	awaitWorkers       = 8
)

type Logger interface {
	Logf(format string, args ...any)
}

type relayerAPI interface {
	PacketStatuses(ctx context.Context, chainID, txHash string) ([]*relayerv2.PacketStatus, error)
}

type catalystTx struct {
	hash    string
	lookups int
}

// catalystPacket is one discovered packet, or a lookup that failed after every attempt.
// err is set only for a failed lookup; packet is set only when err is nil.
type catalystPacket struct {
	txHash string
	packet e2etest.PacketTx
	err    error
}

// AwaitPacketsFromCatalyst queries the relayer until each broadcast transaction
// is indexed and waits for every packet it emitted to stay succeeded.
func AwaitPacketsFromCatalyst(
	ctx context.Context,
	route e2etest.Route,
	relayer *ibccli.Relayer,
	txs []*catalystevm.SentTx,
	relayCompletionTimeout time.Duration,
	logger Logger,
) error {
	const expect = relayerv2.PacketState_PACKET_STATE_SUCCEEDED

	// override default wait policy not to hammer relayer GRPC
	// and give enough time to await for the relay
	waitPolicy := ibccli.WaitPolicy{
		StatusPoll:       intervalPacketPoll,
		CompletionBudget: relayCompletionTimeout,
	}

	packetsCh := discoverCatalystPackets(ctx, route, relayer, txs, logger)

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		errs    []error
		awaited int
	)

	start := time.Now()

	for range awaitWorkers {
		wg.Go(func() {
			for item := range packetsCh {
				if item.err != nil {
					logger.Logf("AwaitPacketsFromCatalyst[%s]: tx %q %v", route.Source, item.txHash, item.err)
					mu.Lock()
					errs = append(errs, item.err)
					mu.Unlock()
					continue
				}

				_, err := e2etest.AwaitStateWithPolicy(ctx, relayer, item.packet, expect, waitPolicy)
				mu.Lock()
				awaited++
				if err != nil {
					errs = append(errs, err)
				}
				if awaited%100 == 0 {
					logger.Logf("AwaitPacketsFromCatalyst[%s]: %d packets succeeded", route.Source, awaited)
				}
				mu.Unlock()
			}
		})
	}

	wg.Wait()

	logger.Logf(
		"AwaitPacketsFromCatalyst[%s]: %d packets awaited in %s",
		route.Source,
		awaited,
		time.Since(start),
	)

	return errors.Join(errs...)
}

// given N catalyst tx, discover all packets emitted by them
// discovers up to maxLookupsPerPoll packets per poll do avoid scanning
// too recent packets (they need some time to be indexed)
func discoverCatalystPackets(
	ctx context.Context,
	route e2etest.Route,
	relayer relayerAPI,
	catalystTxs []*catalystevm.SentTx,
	logger Logger,
) <-chan catalystPacket {
	pendingTxs := txsFromCatalyst(catalystTxs, logger)
	out := make(chan catalystPacket, maxLookupsPerPoll)

	go func() {
		defer close(out)

		if len(pendingTxs) == 0 {
			return
		}

		ticker := time.NewTicker(intervalPacketPoll)
		defer ticker.Stop()

		discovered := 0

		for len(pendingTxs) > 0 {
			logger.Logf(
				"discoverCatalystPackets[%s]: %d txs more to fetch from the relayer",
				route.Source,
				len(pendingTxs),
			)

			if err := ctx.Err(); err != nil {
				out <- catalystPacket{err: errNotIndexedMany(len(pendingTxs), err)}
				return
			}

			limit := min(len(pendingTxs), maxLookupsPerPoll)
			retry := make([]catalystTx, 0, limit)
			for _, tx := range pendingTxs[:limit] {
				txPackets, err := relayer.PacketStatuses(ctx, string(route.Source), tx.hash)
				if err != nil && ctx.Err() != nil {
					out <- catalystPacket{err: errNotIndexedMany(len(pendingTxs), ctx.Err())}
					return
				}

				// allow up to maxLookupAttempts lookups for each src tx
				if err != nil || len(txPackets) == 0 {
					tx.lookups++
					if tx.lookups >= maxLookupAttempts {
						out <- catalystPacket{
							txHash: tx.hash,
							err:    errNotIndexedTx(tx.hash, err),
						}
						continue
					}
					retry = append(retry, tx)
					continue
				}

				for _, status := range txPackets {
					discovered++
					out <- catalystPacket{
						txHash: tx.hash,
						packet: e2etest.PacketTx{
							RouteID:        route.ID,
							Source:         route.Source,
							SourceClientID: status.GetSourceClientId(),
							SourceTxHash:   tx.hash,
							Sequence:       status.GetSequenceNumber(),
						},
					}
				}
			}

			pendingTxs = append(pendingTxs[limit:], retry...)
			if len(pendingTxs) == 0 {
				break
			}

			select {
			case <-ctx.Done():
				out <- catalystPacket{err: errNotIndexedMany(len(pendingTxs), ctx.Err())}
				return
			case <-ticker.C:
			}
		}

		logger.Logf(
			"discoverCatalystPackets[%s]: %d packets discovered",
			route.Source,
			discovered,
		)
	}()

	return out
}

func txsFromCatalyst(catalystTxs []*catalystevm.SentTx, logger Logger) []catalystTx {
	txs := make([]catalystTx, 0, len(catalystTxs))
	for _, tx := range catalystTxs {
		// should not happen
		if tx == nil {
			continue
		}

		if tx.SendTransactionErr != nil {
			logger.Logf("tx %s wasn't broadcasted: %v", tx.TxHash.Hex(), tx.SendTransactionErr)
		}

		txs = append(txs, catalystTx{
			hash:    tx.TxHash.Hex(),
			lookups: 0,
		})
	}

	return txs
}

func errNotIndexedTx(hash string, err error) error {
	if err != nil {
		return fmt.Errorf("tx %s: %w", hash, err)
	}
	return fmt.Errorf("tx %s: not indexed after %d queries", hash, maxLookupAttempts)
}

func errNotIndexedMany(n int, err error) error {
	return fmt.Errorf("await catalyst packets: %d txs not indexed: %w", n, err)
}
