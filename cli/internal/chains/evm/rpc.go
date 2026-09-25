// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

// rpcClient adds eth_getProof to ethclient on the same connection.
type rpcClient struct {
	*ethclient.Client
}

var _ ETHClient = (*rpcClient)(nil)

func newRPCClient(client *rpc.Client) *rpcClient {
	return &rpcClient{Client: ethclient.NewClient(client)}
}

// proofResult is the part of an eth_getProof result the light client needs.
type proofResult struct {
	AccountProof []hexutil.Bytes      `json:"accountProof"`
	StorageProof []storageProofResult `json:"storageProof"`
}

type storageProofResult struct {
	Proof []hexutil.Bytes `json:"proof"`
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
