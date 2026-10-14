// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"math/big"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besumsgs"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/cli/besu"
)

// SealedHeader returns the Besu QBFT header at height.
func (c *Client) SealedHeader(ctx context.Context, height uint64) (*besu.ParsedHeader, error) {
	header, err := besu.ReadSealedHeader(ctx, c.eth, new(big.Int).SetUint64(height))
	if err != nil {
		return nil, errors.Wrapf(err, "chain %s", c.chainID)
	}

	return header, nil
}

// BesuQBFTClientState reads clientID's Besu QBFT light client state.
func (c *Client) BesuQBFTClientState(
	ctx context.Context,
	clientID string,
) (besumsgs.IBesuLightClientMsgsClientState, error) {
	lightClientAddr, err := c.router.GetClient(&bind.CallOpts{Context: ctx}, clientID)
	if err != nil {
		return besumsgs.IBesuLightClientMsgsClientState{}, errors.Wrapf(
			err, "resolving light client address for %q on chain %s", clientID, c.chainID,
		)
	}

	state, err := besu.ReadClientState(ctx, c.eth, lightClientAddr)
	if err != nil {
		return besumsgs.IBesuLightClientMsgsClientState{}, errors.Wrapf(
			err, "client %q on chain %s", clientID, c.chainID,
		)
	}

	return state, nil
}
