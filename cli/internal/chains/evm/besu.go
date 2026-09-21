// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"math/big"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besuerrors"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besumsgs"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besuqbft"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient/gethclient"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/cli/besu"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

// AccountProof is an eth_getProof result for one account at one height: the
// account proof nodes against the block's state root and one storage proof
// per requested slot, in request order.
type AccountProof struct {
	AccountProof  [][]byte
	StorageProofs []StorageProof
}

// StorageProof is one storage slot's value and trie proof nodes from
// eth_getProof. Value is zero for an absent slot.
type StorageProof struct {
	Key   [32]byte
	Value *big.Int
	Proof [][]byte
}

// SealedHeader returns the Besu QBFT header at height, parsed from the node's
// sealed RLP. Validators come from extraData.
func (c *Client) SealedHeader(ctx context.Context, height uint64) (*besu.Header, error) {
	header, err := c.eth.HeaderByNumber(ctx, heightToBigInt(height))
	if err != nil {
		return nil, errors.Wrapf(err, "getting header for height %d on chain %s", height, c.chainID)
	}

	sealed, err := besu.ParseSealedHeader(header)
	if err != nil {
		return nil, errors.Wrapf(err, "header %d on chain %s is not a Besu QBFT header", height, c.chainID)
	}

	if height != v2.LatestBlock && sealed.Height != height {
		return nil, errors.Errorf("chain %s returned header %d for height %d", c.chainID, sealed.Height, height)
	}

	return sealed, nil
}

// GetRouterProof proves the router account and the requested storage slots at
// height via eth_getProof, matching storage proofs to slots by key.
func (c *Client) GetRouterProof(ctx context.Context, height uint64, slots [][32]byte) (AccountProof, error) {
	keys := make([]string, len(slots))
	for i, slot := range slots {
		keys[i] = common.Hash(slot).Hex()
	}

	result, err := c.eth.GetProof(ctx, c.routerAddress, keys, heightToBigInt(height))
	if err != nil {
		return AccountProof{}, errors.Wrapf(err, "getting router proof at height %d on chain %s", height, c.chainID)
	}

	proof, err := accountProofFromResult(result, slots)
	if err != nil {
		return AccountProof{}, errors.Wrapf(err, "router proof at height %d on chain %s", height, c.chainID)
	}

	return proof, nil
}

// accountProofFromResult converts an eth_getProof result, requiring a
// well-formed storage proof for every requested slot regardless of response
// order.
func accountProofFromResult(result *gethclient.AccountResult, slots [][32]byte) (AccountProof, error) {
	byKey := make(map[[32]byte]StorageProof, len(result.StorageProof))

	for i, storage := range result.StorageProof {
		key := common.BytesToHash(common.FromHex(storage.Key))

		if storage.Value == nil || storage.Value.Sign() < 0 || storage.Value.BitLen() > 256 {
			return AccountProof{}, errors.Errorf("storage proof %d has an invalid value", i)
		}

		if _, dup := byKey[key]; dup {
			return AccountProof{}, errors.Errorf("duplicate storage proof for slot %s", key)
		}

		nodes, err := decodeProofNodes(storage.Proof)
		if err != nil {
			return AccountProof{}, errors.Wrapf(err, "storage proof for slot %s", key)
		}
		byKey[key] = StorageProof{Key: key, Value: storage.Value, Proof: nodes}
	}

	proofs := make([]StorageProof, len(slots))

	for i, slot := range slots {
		proof, ok := byKey[slot]
		if !ok {
			return AccountProof{}, errors.Errorf("no storage proof returned for slot %s", common.Hash(slot))
		}

		proofs[i] = proof
	}

	accountNodes, err := decodeProofNodes(result.AccountProof)
	if err != nil {
		return AccountProof{}, errors.Wrap(err, "account proof")
	}

	return AccountProof{
		AccountProof:  accountNodes,
		StorageProofs: proofs,
	}, nil
}

func decodeProofNodes(nodes []string) ([][]byte, error) {
	out := make([][]byte, len(nodes))
	for i, node := range nodes {
		decoded, err := hexutil.Decode(node)
		if err != nil {
			return nil, errors.Wrapf(err, "node %d", i)
		}
		if len(decoded) == 0 {
			return nil, errors.Errorf("node %d is empty", i)
		}
		out[i] = decoded
	}

	return out, nil
}

// GetBesuQBFTClientState reads and decodes clientID's Besu QBFT light
// client state.
func (c *Client) GetBesuQBFTClientState(
	ctx context.Context,
	clientID string,
) (besumsgs.IBesuLightClientMsgsClientState, error) {
	lightClient, err := c.besuQBFTClient(ctx, clientID)
	if err != nil {
		return besumsgs.IBesuLightClientMsgsClientState{}, err
	}

	raw, err := lightClient.GetClientState(&bind.CallOpts{Context: ctx})
	if err != nil {
		return besumsgs.IBesuLightClientMsgsClientState{}, errors.Wrapf(
			err,
			"querying client state for client %q on chain %s",
			clientID,
			c.chainID,
		)
	}

	state, err := besumsgs.NewBindings().UnpackClientState(raw)
	if err != nil {
		return besumsgs.IBesuLightClientMsgsClientState{}, errors.Wrapf(
			err,
			"client %q on chain %s",
			clientID,
			c.chainID,
		)
	}

	return state, nil
}

// GetBesuQBFTConsensusStateHash reads the consensus state hash clientID's Besu
// QBFT light client stores at height.
func (c *Client) GetBesuQBFTConsensusStateHash(
	ctx context.Context,
	clientID string,
	height uint64,
) ([32]byte, error) {
	lightClient, err := c.besuQBFTClient(ctx, clientID)
	if err != nil {
		return [32]byte{}, err
	}

	hash, err := lightClient.GetConsensusStateHash(&bind.CallOpts{Context: ctx}, height)
	if err != nil {
		if isConsensusStateNotFound(err) {
			return [32]byte{}, errors.Wrapf(
				ErrConsensusStateNotFound, "client %q on chain %s at height %d", clientID, c.chainID, height,
			)
		}

		return [32]byte{}, errors.Wrapf(
			err, "querying consensus state hash at height %d for client %q on chain %s", height, clientID, c.chainID,
		)
	}

	return hash, nil
}

func (c *Client) besuQBFTClient(ctx context.Context, clientID string) (*besuqbft.ContractCaller, error) {
	lightClientAddr, err := c.router.GetClient(&bind.CallOpts{Context: ctx}, clientID)
	if err != nil {
		return nil, errors.Wrapf(err, "resolving light client address for %q on chain %s", clientID, c.chainID)
	}

	lightClient, err := besuqbft.NewContractCaller(lightClientAddr, c.eth)
	if err != nil {
		return nil, errors.Wrapf(err, "binding besu qbft light client %q on chain %s", clientID, c.chainID)
	}

	return lightClient, nil
}

var besuErrorBindings = besuerrors.NewBindings()

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

	decoded, decodeErr := besuErrorBindings.UnpackError(revert)
	if decodeErr != nil {
		return false
	}
	_, ok = decoded.(*besuerrors.BindingsConsensusStateNotFound)
	return ok
}
