// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/ethclient/gethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

// rpcClient combines ethclient with eth_getProof on the same connection.
// go-ethereum exposes the latter only through gethclient.
type rpcClient struct {
	*ethclient.Client
	proofs *gethclient.Client
}

var _ ETHClient = (*rpcClient)(nil)

func newRPCClient(client *rpc.Client) *rpcClient {
	return &rpcClient{Client: ethclient.NewClient(client), proofs: gethclient.New(client)}
}

func (c *rpcClient) GetProof(
	ctx context.Context,
	account common.Address,
	keys []string,
	blockNumber *big.Int,
) (*gethclient.AccountResult, error) {
	return c.proofs.GetProof(ctx, account, keys, blockNumber)
}
