package testapp

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/observe"

	ethereum "github.com/ethereum/go-ethereum"
)

func mustABI(metadata *bind.MetaData) abi.ABI {
	parsed, err := metadata.GetAbi()
	if err != nil {
		panic(fmt.Sprintf("testapp: parse generated contract ABI: %v", err))
	}
	return *parsed
}

func mustBinding[T any](binding T, err error) T {
	if err != nil {
		panic(fmt.Sprintf("testapp: construct generated contract binding: %v", err))
	}
	return binding
}

func send(
	ctx context.Context,
	client *environment.EVM,
	sender evm.Account,
	to common.Address,
	data []byte,
	sequenceFromReceipt func(*types.Receipt) (uint64, bool, error),
) (string, uint64, error) {
	receipt, err := client.BroadcastTx(ctx, sender, &to, data, nil)
	if err != nil {
		return "", 0, fmt.Errorf("testapp: broadcast transaction: %w", err)
	}
	sequence, found, err := sequenceFromReceipt(receipt)
	if err != nil {
		return "", 0, err
	}
	if !found {
		return "", 0, fmt.Errorf(
			"testapp: source transaction %s emitted no application send event",
			receipt.TxHash.Hex(),
		)
	}
	return receipt.TxHash.Hex(), sequence, nil
}

type eventSource struct {
	endpoint endpoint
	contract common.Address
}

func awaitEvent[T any](
	ctx context.Context,
	source eventSource,
	topic common.Hash,
	description string,
	decode func(types.Log) (T, error),
	match func(T) bool,
) (T, error) {
	query := ethereum.FilterQuery{
		FromBlock: big.NewInt(0),
		Addresses: []common.Address{source.contract},
		Topics:    [][]common.Hash{{topic}},
	}
	timing := source.endpoint.chain.Timing()
	return observe.Await(
		ctx,
		timing.CompletionBudget,
		timing.PollInterval,
		description,
		func(ctx context.Context) (T, bool, error) {
			var zero T
			logs, err := source.endpoint.evm.Logs(ctx, query)
			if err != nil {
				return zero, false, err
			}
			for _, log := range logs {
				event, err := decode(log)
				if err != nil {
					return zero, true, err
				}
				if match(event) {
					return event, true, nil
				}
			}
			return zero, false, nil
		},
	)
}

func awaitPacketEvent[T any](
	ctx context.Context,
	source eventSource,
	topic common.Hash,
	application string,
	decode func(types.Log) (T, error),
	packet Packet,
	key func(T) (string, *big.Int),
) (T, error) {
	description := fmt.Sprintf("%s delivery for packet %s", application, packet.reference())
	return awaitEvent(ctx, source, topic, description, decode, func(event T) bool {
		routeID, sequence := key(event)
		return routeID == string(packet.RouteID) && sequence.Uint64() == packet.Sequence
	})
}
