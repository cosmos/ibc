// SPDX-License-Identifier: Apache-2.0

package relayerload

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"golang.org/x/sync/errgroup"

	relayerv2 "github.com/cosmos/ibc/cli/api/v2/relayer"
	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
)

// latencyFetchers bounds concurrent receipt and header requests per Chain.
const latencyFetchers = 16

// chainReader reads the chain data relay latency comes from, as
// environment.EVM does: its receipt read already waits within the Chain's
// completion budget.
type chainReader interface {
	AwaitTransactionReceipt(ctx context.Context, hash common.Hash) (*types.Receipt, error)
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
}

// retryingHeaders repeats a header read the node did not answer, as a send
// does, within the Chain's completion budget.
type retryingHeaders struct {
	chainReader
	wait evm.TransactionWait
}

func retrying(chain chainReader, timing environment.Timing) chainReader {
	return retryingHeaders{chainReader: chain, wait: timing.TransactionWait()}
}

func (r retryingHeaders) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	return evm.Retry(ctx, r.wait, func(ctx context.Context) (*types.Header, error) {
		return r.chainReader.HeaderByNumber(ctx, number)
	})
}

// relayLatencies measures every succeeded packet's relay from block
// timestamps: send to receive is the destination block that included the
// receive transaction minus the source block that included the transfer;
// receive to acknowledgement and send to acknowledgement end at the source
// block that included the acknowledgement. Block timestamps have one-second
// resolution, so precision is the Chains' block time. A failed read skips the
// packets that need it and is returned alongside the other packets' samples.
func relayLatencies(
	ctx context.Context,
	source, destination chainReader,
	packets []*trackedPacket,
) (sendToRecv, recvToAck, sendToAck []time.Duration, _ error) {
	var succeeded []*trackedPacket
	recvTxs, ackTxs := map[string]struct{}{}, map[string]struct{}{}
	for _, p := range packets {
		if p.state() != relayerv2.PacketState_PACKET_STATE_SUCCEEDED {
			continue
		}
		succeeded = append(succeeded, p)
		recvTxs[p.status.GetRecvTx().GetTxHash()] = struct{}{}
		ackTxs[p.status.GetAckTx().GetTxHash()] = struct{}{}
	}
	if len(succeeded) == 0 {
		return nil, nil, nil, nil
	}

	inclusionBlock := func(chain chainReader) func(context.Context, string) (uint64, error) {
		return func(ctx context.Context, hash string) (uint64, error) {
			receipt, receiptErr := chain.AwaitTransactionReceipt(ctx, common.HexToHash(hash))
			if receiptErr != nil {
				return 0, fmt.Errorf("receipt %s: %w", hash, receiptErr)
			}
			return receipt.BlockNumber.Uint64(), nil
		}
	}
	recvBlocks, recvErr := fetchAll(ctx, recvTxs, inclusionBlock(destination))
	ackBlocks, ackErr := fetchAll(ctx, ackTxs, inclusionBlock(source))

	sourceBlocks, destinationBlocks := map[uint64]struct{}{}, map[uint64]struct{}{}
	for _, p := range succeeded {
		sourceBlocks[p.block] = struct{}{}
		if block, ok := ackBlocks[p.status.GetAckTx().GetTxHash()]; ok {
			sourceBlocks[block] = struct{}{}
		}
		if block, ok := recvBlocks[p.status.GetRecvTx().GetTxHash()]; ok {
			destinationBlocks[block] = struct{}{}
		}
	}
	blockTime := func(chain chainReader) func(context.Context, uint64) (time.Time, error) {
		return func(ctx context.Context, number uint64) (time.Time, error) {
			header, headerErr := chain.HeaderByNumber(ctx, new(big.Int).SetUint64(number))
			if headerErr != nil {
				return time.Time{}, fmt.Errorf("header %d: %w", number, headerErr)
			}
			return time.Unix(int64(header.Time), 0), nil
		}
	}
	sourceTimes, sourceErr := fetchAll(ctx, sourceBlocks, blockTime(source))
	destinationTimes, destinationErr := fetchAll(ctx, destinationBlocks, blockTime(destination))

	for _, p := range succeeded {
		sent, sentOK := sourceTimes[p.block]
		received, receivedOK := timeOf(destinationTimes, recvBlocks, p.status.GetRecvTx().GetTxHash())
		acked, ackedOK := timeOf(sourceTimes, ackBlocks, p.status.GetAckTx().GetTxHash())
		if !sentOK || !receivedOK || !ackedOK {
			continue
		}
		sendToRecv = append(sendToRecv, max(0, received.Sub(sent)))
		recvToAck = append(recvToAck, max(0, acked.Sub(received)))
		sendToAck = append(sendToAck, max(0, acked.Sub(sent)))
	}
	return sendToRecv, recvToAck, sendToAck, errors.Join(recvErr, ackErr, sourceErr, destinationErr)
}

// timeOf is the timestamp of the block that included the transaction hash.
func timeOf(times map[uint64]time.Time, blocks map[string]uint64, hash string) (time.Time, bool) {
	block, ok := blocks[hash]
	if !ok {
		return time.Time{}, false
	}
	at, ok := times[block]
	return at, ok
}

// fetchAll fetches every key with bounded concurrency. It returns the values
// fetched and the first error; a failed key is absent from the values.
func fetchAll[K comparable, V any](
	ctx context.Context,
	keys map[K]struct{},
	fetch func(context.Context, K) (V, error),
) (map[K]V, error) {
	var (
		mu       sync.Mutex
		values   = make(map[K]V, len(keys))
		fetchers errgroup.Group
	)
	fetchers.SetLimit(latencyFetchers)
	for key := range keys {
		fetchers.Go(func() error {
			value, err := fetch(ctx, key)
			if err != nil {
				return err
			}
			mu.Lock()
			defer mu.Unlock()
			values[key] = value
			return nil
		})
	}
	return values, fetchers.Wait()
}
