// SPDX-License-Identifier: Apache-2.0

package besu

import (
	"context"
	"fmt"
	"math/big"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besumsgs"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besuqbft"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// HeaderReader reads block headers from a node.
type HeaderReader interface {
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
}

// ReadSealedHeader reads and parses the Besu QBFT header at number, or the
// head when number is nil.
func ReadSealedHeader(ctx context.Context, reader HeaderReader, number *big.Int) (*ParsedHeader, error) {
	at := "latest"
	if number != nil {
		at = number.String()
	}

	header, err := reader.HeaderByNumber(ctx, number)
	if err != nil {
		return nil, fmt.Errorf("getting header %s: %w", at, err)
	}

	parsed, err := ParseSealedHeader(header)
	if err != nil {
		return nil, fmt.Errorf("header %s is not a Besu QBFT header: %w", at, err)
	}

	if number != nil && (!number.IsUint64() || parsed.Height != number.Uint64()) {
		return nil, fmt.Errorf("node returned header %d for height %v", parsed.Height, number)
	}

	return parsed, nil
}

// ReadClientState reads the state of the Besu QBFT light client at address.
func ReadClientState(
	ctx context.Context,
	caller bind.ContractCaller,
	address common.Address,
) (besumsgs.IBesuLightClientMsgsClientState, error) {
	lightClient, err := besuqbft.NewContractCaller(address, caller)
	if err != nil {
		return besumsgs.IBesuLightClientMsgsClientState{}, err
	}

	raw, err := lightClient.GetClientState(&bind.CallOpts{Context: ctx})
	if err != nil {
		return besumsgs.IBesuLightClientMsgsClientState{}, fmt.Errorf("querying client state: %w", err)
	}

	state, err := besumsgs.NewBindings().UnpackClientState(raw)
	if err != nil {
		return besumsgs.IBesuLightClientMsgsClientState{}, fmt.Errorf(
			"%s is not a besu-qbft light client: %w", address, err,
		)
	}

	return state, nil
}
