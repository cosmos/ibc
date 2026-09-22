// SPDX-License-Identifier: Apache-2.0

// Package besu reads what Besu light clients need from an EVM chain: sealed
// headers of a Besu chain and the state of Besu light-client contracts.
package besu

import (
	"context"
	"math/big"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besuerrors"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besumsgs"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besuqbft"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/cli/besu"
	chainsevm "github.com/cosmos/ibc/cli/internal/chains/evm"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

// ErrConsensusStateNotFound reports that a light client stores nothing at the
// requested height.
var ErrConsensusStateNotFound = errors.New("consensus state not found")

// ETHClient go-ethereum methods used by the Besu client.
type ETHClient interface {
	bind.ContractCaller
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
}

// Client is an EVM chain client that also reads Besu headers and Besu
// light-client contracts.
type Client struct {
	*chainsevm.Client

	eth    ETHClient
	router *ics26router.ContractCaller
}

// New wraps an EVM client, sharing its connection.
func New(evmClient *chainsevm.Client) (*Client, error) {
	eth := evmClient.ETH()

	router, err := ics26router.NewContractCaller(evmClient.RouterAddress(), eth)
	if err != nil {
		return nil, errors.Wrap(err, "creating ics26 router binding")
	}

	return &Client{Client: evmClient, eth: eth, router: router}, nil
}

// SealedHeader returns the Besu QBFT header at height, parsed from the node's
// sealed RLP. Validators come from extraData.
func (c *Client) SealedHeader(ctx context.Context, height uint64) (*besu.Header, error) {
	header, err := c.eth.HeaderByNumber(ctx, blockNumber(height))
	if err != nil {
		return nil, errors.Wrapf(err, "getting header for height %d on chain %s", height, c.ChainID())
	}

	sealed, err := besu.ParseSealedHeader(header)
	if err != nil {
		return nil, errors.Wrapf(err, "header %d on chain %s is not a Besu QBFT header", height, c.ChainID())
	}

	if height != v2.LatestBlock && sealed.Height != height {
		return nil, errors.Errorf("chain %s returned header %d for height %d", c.ChainID(), sealed.Height, height)
	}

	return sealed, nil
}

// ClientState reads and decodes clientID's Besu QBFT light client state.
func (c *Client) ClientState(ctx context.Context, clientID string) (besumsgs.IBesuLightClientMsgsClientState, error) {
	lightClient, err := c.lightClient(ctx, clientID)
	if err != nil {
		return besumsgs.IBesuLightClientMsgsClientState{}, err
	}

	raw, err := lightClient.GetClientState(&bind.CallOpts{Context: ctx})
	if err != nil {
		return besumsgs.IBesuLightClientMsgsClientState{}, errors.Wrapf(
			err,
			"querying client state for client %q on chain %s",
			clientID,
			c.ChainID(),
		)
	}

	state, err := besumsgs.NewBindings().UnpackClientState(raw)
	if err != nil {
		return besumsgs.IBesuLightClientMsgsClientState{}, errors.Wrapf(
			err,
			"client %q on chain %s",
			clientID,
			c.ChainID(),
		)
	}

	return state, nil
}

// ConsensusStateHash reads the consensus state hash clientID's Besu QBFT
// light client stores at height.
func (c *Client) ConsensusStateHash(ctx context.Context, clientID string, height uint64) ([32]byte, error) {
	lightClient, err := c.lightClient(ctx, clientID)
	if err != nil {
		return [32]byte{}, err
	}

	hash, err := lightClient.GetConsensusStateHash(&bind.CallOpts{Context: ctx}, height)
	if err != nil {
		if isConsensusStateNotFound(err) {
			return [32]byte{}, errors.Wrapf(
				ErrConsensusStateNotFound, "client %q on chain %s at height %d", clientID, c.ChainID(), height,
			)
		}

		return [32]byte{}, errors.Wrapf(
			err, "querying consensus state hash at height %d for client %q on chain %s", height, clientID, c.ChainID(),
		)
	}

	return hash, nil
}

func (c *Client) lightClient(ctx context.Context, clientID string) (*besuqbft.ContractCaller, error) {
	lightClientAddr, err := c.router.GetClient(&bind.CallOpts{Context: ctx}, clientID)
	if err != nil {
		return nil, errors.Wrapf(err, "resolving light client address for %q on chain %s", clientID, c.ChainID())
	}

	lightClient, err := besuqbft.NewContractCaller(lightClientAddr, c.eth)
	if err != nil {
		return nil, errors.Wrapf(err, "binding besu qbft light client %q on chain %s", clientID, c.ChainID())
	}

	return lightClient, nil
}

var errorBindings = besuerrors.NewBindings()

// isConsensusStateNotFound recognizes the light client's
// ConsensusStateNotFound(uint64) revert in a JSON-RPC error's data.
func isConsensusStateNotFound(err error) bool {
	var dataErr interface{ ErrorData() any }
	if !errors.As(err, &dataErr) {
		return false
	}

	data, ok := dataErr.ErrorData().(string)
	if !ok {
		return false
	}

	revert, decodeErr := hexutil.Decode(data)
	if decodeErr != nil || len(revert) < 4 {
		return false
	}

	decoded, decodeErr := errorBindings.UnpackError(revert)
	if decodeErr != nil {
		return false
	}
	_, ok = decoded.(*besuerrors.BindingsConsensusStateNotFound)
	return ok
}

func blockNumber(height uint64) *big.Int {
	switch height {
	case v2.LatestBlock:
		return big.NewInt(rpc.LatestBlockNumber.Int64())
	case v2.FinalizedBlock:
		return big.NewInt(rpc.FinalizedBlockNumber.Int64())
	default:
		return new(big.Int).SetUint64(height)
	}
}
