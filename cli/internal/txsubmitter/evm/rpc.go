// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient"
)

type rpcClient struct{ *ethclient.Client }

// Use RPC-reported hashes; geth's Header.Hash() differs on Besu QBFT.
func (c rpcClient) BlockHashByNumber(ctx context.Context, number uint64) (common.Hash, error) {
	var block *struct {
		Hash common.Hash `json:"hash"`
	}
	if err := c.Client.Client().
		CallContext(ctx, &block, "eth_getBlockByNumber", hexutil.EncodeUint64(number), false); err != nil {
		return common.Hash{}, err
	}
	if block == nil || block.Hash == (common.Hash{}) {
		return common.Hash{}, ethereum.NotFound
	}
	return block.Hash, nil
}
