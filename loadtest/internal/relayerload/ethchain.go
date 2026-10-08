// SPDX-License-Identifier: Apache-2.0

package relayerload

import (
	"context"
	"fmt"
	"math/big"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/cosmos/ibc/harness/environment"
)

// ethChain is the generator's own RPC connection to a Chain: HTTP for calls
// and transaction submission, WebSocket for new heads. The harness's EVM
// handle waits for every receipt, which an open-loop generator must not do.
type ethChain struct {
	id     environment.ChainID
	evmID  *big.Int
	client *ethclient.Client
	ws     *ethclient.Client
}

func dialChain(ctx context.Context, chain *environment.Chain) (*ethChain, error) {
	if chain.WSURL() == "" {
		return nil, fmt.Errorf("chain %q has no WebSocket endpoint for head subscriptions", chain.ID())
	}
	client, err := ethclient.DialContext(ctx, chain.RPCURL())
	if err != nil {
		return nil, fmt.Errorf("dial chain %q RPC: %w", chain.ID(), err)
	}
	ws, err := ethclient.DialContext(ctx, chain.WSURL())
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("dial chain %q WebSocket: %w", chain.ID(), err)
	}
	return &ethChain{
		id:     chain.ID(),
		evmID:  new(big.Int).SetUint64(chain.EVMChainID()),
		client: client,
		ws:     ws,
	}, nil
}

func (c *ethChain) Close() {
	c.client.Close()
	c.ws.Close()
}

func (c *ethChain) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	return c.client.PendingNonceAt(ctx, account)
}

func (c *ethChain) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	return c.client.SendTransaction(ctx, tx)
}

func (c *ethChain) BlockReceipts(ctx context.Context, number uint64) ([]*types.Receipt, error) {
	return c.client.BlockReceipts(ctx, rpc.BlockNumberOrHashWithNumber(rpc.BlockNumber(number)))
}

func (c *ethChain) TransactionReceipt(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
	return c.client.TransactionReceipt(ctx, hash)
}

func (c *ethChain) SubscribeNewHead(ctx context.Context, heads chan<- *types.Header) (ethereum.Subscription, error) {
	return c.ws.SubscribeNewHead(ctx, heads)
}

// estimateGas estimates msg against the pending state, falling back to the
// latest block: Besu can reject a pending estimate whose future timestamp
// trips a time check (the harness's own sender does the same).
func (c *ethChain) estimateGas(ctx context.Context, msg ethereum.CallMsg) (uint64, error) {
	gas, err := c.client.EstimateGas(ctx, msg)
	if err == nil {
		return gas, nil
	}
	return c.client.EstimateGasAtBlock(ctx, msg, nil)
}

func (c *ethChain) suggestTipCap(ctx context.Context) (*big.Int, error) {
	tip, err := c.client.SuggestGasTipCap(ctx)
	if err != nil {
		return nil, fmt.Errorf("suggest gas tip cap: %w", err)
	}
	if tip == nil || tip.Sign() <= 0 {
		tip = big.NewInt(fallbackTipCapWei)
	}
	return tip, nil
}
