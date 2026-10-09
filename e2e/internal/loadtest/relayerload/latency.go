// SPDX-License-Identifier: Apache-2.0

package relayerload

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"golang.org/x/sync/errgroup"

	relayerv2 "github.com/cosmos/ibc/cli/api/v2/relayer"
)

// latencyFetchers bounds concurrent receipt and header requests per Chain.
const latencyFetchers = 16

// chainReader reads the chain data relay latency comes from.
type chainReader interface {
	TransactionReceipt(ctx context.Context, hash common.Hash) (*types.Receipt, error)
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
}

// relayLatencies measures every succeeded packet's relay from block
// timestamps: send to receive is the destination block that included the
// receive transaction minus the source block that included the transfer;
// receive to acknowledgement and send to acknowledgement end at the source
// block that included the acknowledgement. Block timestamps have one-second
// resolution, so precision is the Chains' block time.
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
			receipt, receiptErr := chain.TransactionReceipt(ctx, common.HexToHash(hash))
			if receiptErr != nil {
				return 0, fmt.Errorf("receipt %s: %w", hash, receiptErr)
			}
			return receipt.BlockNumber.Uint64(), nil
		}
	}
	recvBlocks, err := fetchAll(ctx, recvTxs, inclusionBlock(destination))
	if err != nil {
		return nil, nil, nil, err
	}
	ackBlocks, err := fetchAll(ctx, ackTxs, inclusionBlock(source))
	if err != nil {
		return nil, nil, nil, err
	}

	sourceBlocks, destinationBlocks := map[uint64]struct{}{}, map[uint64]struct{}{}
	for _, p := range succeeded {
		sourceBlocks[p.sendBlock] = struct{}{}
		sourceBlocks[ackBlocks[p.status.GetAckTx().GetTxHash()]] = struct{}{}
		destinationBlocks[recvBlocks[p.status.GetRecvTx().GetTxHash()]] = struct{}{}
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
	sourceTimes, err := fetchAll(ctx, sourceBlocks, blockTime(source))
	if err != nil {
		return nil, nil, nil, err
	}
	destinationTimes, err := fetchAll(ctx, destinationBlocks, blockTime(destination))
	if err != nil {
		return nil, nil, nil, err
	}

	for _, p := range succeeded {
		sent := sourceTimes[p.sendBlock]
		received := destinationTimes[recvBlocks[p.status.GetRecvTx().GetTxHash()]]
		acked := sourceTimes[ackBlocks[p.status.GetAckTx().GetTxHash()]]
		sendToRecv = append(sendToRecv, max(0, received.Sub(sent)))
		recvToAck = append(recvToAck, max(0, acked.Sub(received)))
		sendToAck = append(sendToAck, max(0, acked.Sub(sent)))
	}
	return sendToRecv, recvToAck, sendToAck, nil
}

// fetchAll fetches every key with bounded concurrency; the first error
// cancels the rest.
func fetchAll[K comparable, V any](
	ctx context.Context,
	keys map[K]struct{},
	fetch func(context.Context, K) (V, error),
) (map[K]V, error) {
	group, ctx := errgroup.WithContext(ctx)
	group.SetLimit(latencyFetchers)
	var mu sync.Mutex
	values := make(map[K]V, len(keys))
	for key := range keys {
		group.Go(func() error {
			value, err := fetch(ctx, key)
			if err != nil {
				return err
			}
			mu.Lock()
			values[key] = value
			mu.Unlock()
			return nil
		})
	}
	return values, group.Wait()
}
