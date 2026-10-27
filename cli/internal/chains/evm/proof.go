// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/pkg/errors"
)

// RouterProof is an eth_getProof result for the router at one height: the
// account proof nodes against the block's state root and the storage proof
// nodes of each requested slot, in request order.
type RouterProof struct {
	AccountProof  [][]byte
	StorageProofs [][][]byte
}

// rpcClient adds eth_getProof to ethclient on the same connection.
type rpcClient struct {
	*ethclient.Client
}

var _ ETHClient = (*rpcClient)(nil)

// proofResult is the part of an eth_getProof result the light client needs.
type proofResult struct {
	AccountProof []hexutil.Bytes      `json:"accountProof"`
	StorageProof []storageProofResult `json:"storageProof"`
}

// storageProofResult is one slot's proof. Key is the slot as the node
// returns it: Besu drops its leading zeros.
type storageProofResult struct {
	Key   string          `json:"key"`
	Proof []hexutil.Bytes `json:"proof"`
}

// GetRouterProof proves the router account and the requested storage slots at
// height via eth_getProof. The light client verifies the proofs, deriving each
// storage key from the packet itself.
func (c *Client) GetRouterProof(ctx context.Context, height uint64, slots []common.Hash) (RouterProof, error) {
	accountProof, storageProofs, err := c.eth.GetProof(ctx, c.routerAddress, slots, heightToBigInt(height))
	if err != nil {
		return RouterProof{}, errors.Wrapf(err, "getting router proof at height %d on chain %s", height, c.chainID)
	}

	return RouterProof{AccountProof: accountProof, StorageProofs: storageProofs}, nil
}

func newRPCClient(client *rpc.Client) *rpcClient {
	return &rpcClient{Client: ethclient.NewClient(client)}
}

// GetProof calls eth_getProof directly: go-ethereum only offers it through
// gethclient, which imports most of go-ethereum's node internals. Storage
// proofs are matched to keys by the key each carries, since nodes need not
// answer in request order: Besu sorts them by key.
func (c *rpcClient) GetProof(
	ctx context.Context,
	account common.Address,
	keys []common.Hash,
	blockNumber *big.Int,
) ([][]byte, [][][]byte, error) {
	if keys == nil {
		keys = []common.Hash{}
	}
	block := rpc.LatestBlockNumber
	if blockNumber != nil {
		block = rpc.BlockNumber(blockNumber.Int64())
	}

	var result proofResult
	if err := c.Client.Client().CallContext(ctx, &result, "eth_getProof", account, keys, block); err != nil {
		return nil, nil, err
	}

	byKey := make(map[common.Hash][][]byte, len(result.StorageProof))
	for _, storage := range result.StorageProof {
		byKey[common.HexToHash(storage.Key)] = proofNodes(storage.Proof)
	}
	storageProofs := make([][][]byte, len(keys))
	for i, key := range keys {
		proof, ok := byKey[key]
		if !ok {
			return nil, nil, errors.Errorf("no storage proof returned for key %s", key)
		}
		storageProofs[i] = proof
	}

	return proofNodes(result.AccountProof), storageProofs, nil
}

func proofNodes(nodes []hexutil.Bytes) [][]byte {
	out := make([][]byte, len(nodes))
	for i, node := range nodes {
		out[i] = node
	}

	return out
}
