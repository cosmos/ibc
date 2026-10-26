// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc-contracts/packages/go-abigen/besumsgs"
	"github.com/cosmos/ibc-contracts/packages/go-abigen/besuqbft"
	"github.com/cosmos/ibc/cli/besu"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

// SealedHeader returns the sealed Besu header at height, or the head at
// v2.LatestBlock.
func (c *Client) SealedHeader(ctx context.Context, height uint64) (*besu.ParsedHeader, error) {
	var number *big.Int
	if height != v2.LatestBlock {
		number = new(big.Int).SetUint64(height)
	}
	header, err := besu.ReadSealedHeader(ctx, c.eth, number)
	if err != nil {
		return nil, errors.Wrapf(err, "chain %s", c.chainID)
	}

	return header, nil
}

// BesuClientState reads clientID's Besu light client state.
func (c *Client) BesuClientState(
	ctx context.Context,
	clientID string,
) (besumsgs.IBesuLightClientMsgsClientState, error) {
	lightClientAddr, err := c.lightClientAddress(ctx, clientID)
	if err != nil {
		return besumsgs.IBesuLightClientMsgsClientState{}, err
	}

	state, err := besu.ReadClientState(ctx, c.eth, lightClientAddr)
	if err != nil {
		return besumsgs.IBesuLightClientMsgsClientState{}, errors.Wrapf(
			err, "client %q on chain %s", clientID, c.chainID,
		)
	}

	return state, nil
}

// BesuConsensusStateHash returns the hash of the consensus state clientID
// stores at height. The contract reverts when it stores none.
func (c *Client) BesuConsensusStateHash(ctx context.Context, clientID string, height uint64) (common.Hash, error) {
	lightClientAddr, err := c.lightClientAddress(ctx, clientID)
	if err != nil {
		return common.Hash{}, err
	}

	// both Besu light clients share this ABI
	lightClient, err := besuqbft.NewContractCaller(lightClientAddr, c.eth)
	if err != nil {
		return common.Hash{}, err
	}

	hash, err := lightClient.GetConsensusStateHash(&bind.CallOpts{Context: ctx}, height)
	if err != nil {
		return common.Hash{}, errors.Wrapf(
			err, "reading consensus state at height %d of client %q on chain %s", height, clientID, c.chainID,
		)
	}

	return hash, nil
}

func (c *Client) lightClientAddress(ctx context.Context, clientID string) (common.Address, error) {
	addr, err := c.router.GetClient(&bind.CallOpts{Context: ctx}, clientID)
	if err != nil {
		return common.Address{}, errors.Wrapf(
			err, "resolving light client address for %q on chain %s", clientID, c.chainID,
		)
	}

	return addr, nil
}
