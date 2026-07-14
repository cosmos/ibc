package testapp

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/cosmos/ibc/link/e2e/internal/observe"
	"github.com/cosmos/ibc/link/harness/chain/evm"
	"github.com/cosmos/ibc/link/harness/environment"

	ethereum "github.com/ethereum/go-ethereum"
)

func send(
	ctx context.Context,
	client *environment.EVM,
	sender evm.Account,
	to common.Address,
	data []byte,
	contract abi.ABI,
	event string,
) (string, uint64, error) {
	receipt, err := client.BroadcastTx(ctx, sender, &to, data, nil)
	if err != nil {
		return "", 0, fmt.Errorf("testapp: broadcast transaction: %w", err)
	}
	sequence, found, err := sentSequence(receipt, contract, event)
	if err != nil {
		return "", 0, err
	}
	if !found {
		return "", 0, fmt.Errorf(
			"testapp: source transaction %s emitted no %s event",
			receipt.TxHash.Hex(),
			event,
		)
	}
	return receipt.TxHash.Hex(), sequence, nil
}

func sentSequence(receipt *types.Receipt, contract abi.ABI, event string) (uint64, bool, error) {
	definition, ok := contract.Events[event]
	if !ok {
		return 0, false, fmt.Errorf("testapp: ABI has no %s event", event)
	}
	for _, log := range receipt.Logs {
		if len(log.Topics) == 0 || log.Topics[0] != definition.ID {
			continue
		}
		values, err := definition.Inputs.Unpack(log.Data)
		if err != nil {
			return 0, false, fmt.Errorf("testapp: decode %s: %w", event, err)
		}
		if len(values) == 0 {
			return 0, false, fmt.Errorf("testapp: %s event has no sequence field", event)
		}
		sequence, ok := values[0].(*big.Int)
		if !ok {
			return 0, false, fmt.Errorf(
				"testapp: %s sequence field is %T, want *big.Int",
				event,
				values[0],
			)
		}
		return sequence.Uint64(), true, nil
	}
	return 0, false, nil
}

func awaitEvent[T any](
	ctx context.Context,
	chain *environment.Chain,
	client *environment.EVM,
	contract common.Address,
	topic common.Hash,
	description string,
	decode func([]byte) (T, error),
	match func(T) bool,
) (T, error) {
	query := ethereum.FilterQuery{
		FromBlock: big.NewInt(0),
		Addresses: []common.Address{contract},
		Topics:    [][]common.Hash{{topic}},
	}
	timing := chain.Timing()
	return observe.Await(
		ctx,
		timing.CompletionBudget,
		timing.PollInterval,
		description,
		func(ctx context.Context) (T, bool, error) {
			var zero T
			logs, err := client.Logs(ctx, query)
			if err != nil {
				return zero, false, err
			}
			for _, log := range logs {
				event, err := decode(log.Data)
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

func callUint(
	ctx context.Context,
	client *environment.EVM,
	contract abi.ABI,
	address common.Address,
	method string,
	args ...any,
) (*big.Int, error) {
	data, err := contract.Pack(method, args...)
	if err != nil {
		return nil, fmt.Errorf("testapp: pack %s: %w", method, err)
	}
	result, err := client.CallContract(ctx, ethereum.CallMsg{To: &address, Data: data}, nil)
	if err != nil {
		return nil, fmt.Errorf("testapp: call %s on %s: %w", method, address.Hex(), err)
	}
	values, err := contract.Unpack(method, result)
	if err != nil {
		return nil, fmt.Errorf("testapp: decode %s on %s: %w", method, address.Hex(), err)
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("testapp: %s on %s returned no values", method, address.Hex())
	}
	value, ok := values[0].(*big.Int)
	if !ok {
		return nil, fmt.Errorf(
			"testapp: %s on %s returned %T, want *big.Int",
			method,
			address.Hex(),
			values[0],
		)
	}
	return value, nil
}
