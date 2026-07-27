package e2etest

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"

	ethereum "github.com/ethereum/go-ethereum"
)

func mustABI(metadata *bind.MetaData) abi.ABI {
	parsed, err := metadata.GetAbi()
	if err != nil {
		panic(fmt.Sprintf("e2etest: parse generated contract ABI: %v", err))
	}
	return *parsed
}

func mustBinding[T any](binding T, err error) T {
	if err != nil {
		panic(fmt.Sprintf("e2etest: construct generated contract binding: %v", err))
	}
	return binding
}

func deployContract(
	ctx context.Context,
	client *environment.EVM,
	sender evm.Account,
	metadata *bind.MetaData,
	constructorArgs ...any,
) (common.Address, error) {
	parsed := mustABI(metadata)
	payload := common.FromHex(metadata.Bin)
	if len(constructorArgs) > 0 {
		packed, err := parsed.Pack("", constructorArgs...)
		if err != nil {
			return common.Address{}, fmt.Errorf("e2etest: pack constructor: %w", err)
		}
		payload = append(payload, packed...)
	}
	receipt, err := client.BroadcastTx(ctx, sender, nil, payload, nil)
	if err != nil {
		return common.Address{}, fmt.Errorf("e2etest: deploy contract: %w", err)
	}
	if receipt.ContractAddress == (common.Address{}) {
		return common.Address{}, errors.New("e2etest: deployment produced no contract address")
	}
	return receipt.ContractAddress, nil
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
		return "", 0, fmt.Errorf("e2etest: broadcast transaction: %w", err)
	}
	sequence, found, err := sequenceFromReceipt(receipt)
	if err != nil {
		return "", 0, err
	}
	if !found {
		return "", 0, fmt.Errorf(
			"e2etest: source transaction %s emitted no application send event",
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
	return await(
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

func awaitBalance(
	ctx context.Context,
	chain *environment.Chain,
	description string,
	probe func(context.Context) (*big.Int, error),
	want *big.Int,
) error {
	timing := chain.Timing()
	_, err := await(
		ctx,
		timing.CompletionBudget,
		timing.PollInterval,
		description,
		func(ctx context.Context) (struct{}, bool, error) {
			got, err := probe(ctx)
			if err != nil {
				return struct{}{}, false, err
			}
			if got.Cmp(want) != 0 {
				return struct{}{}, false, fmt.Errorf("balance %s, want %s", got, want)
			}
			return struct{}{}, true, nil
		},
	)
	return err
}
