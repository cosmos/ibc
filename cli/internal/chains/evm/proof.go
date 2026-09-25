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

type storageProofResult struct {
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

	if len(storageProofs) != len(slots) {
		return RouterProof{}, errors.Errorf(
			"router proof at height %d on chain %s: %d storage proofs returned for %d slots",
			height, c.chainID, len(storageProofs), len(slots),
		)
	}

	return RouterProof{AccountProof: accountProof, StorageProofs: storageProofs}, nil
}

func newRPCClient(client *rpc.Client) *rpcClient {
	return &rpcClient{Client: ethclient.NewClient(client)}
}

// GetProof calls eth_getProof directly: go-ethereum only offers it through
// gethclient, which imports most of go-ethereum's node internals.
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

	storageProofs := make([][][]byte, len(result.StorageProof))
	for i, storage := range result.StorageProof {
		storageProofs[i] = proofNodes(storage.Proof)
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
