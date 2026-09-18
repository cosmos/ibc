// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"errors"
	"math/big"
	"strconv"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient/gethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"go.opentelemetry.io/otel/metric"

	"github.com/cosmos/ibc/cli/internal/otel"
)

type instrumentation struct {
	Operation metric.Float64Histogram
}

var metrics = otel.RegisterMetrics("evm_client", newInstrumentation)

func newInstrumentation(m metric.Meter) (*instrumentation, error) {
	operation, err := m.Float64Histogram("evm_operation_dur", otel.UnitMilliseconds())
	if err != nil {
		return nil, err
	}

	return &instrumentation{Operation: operation}, nil
}

// meteredClient records every go-ethereum call in evm_operation_dur, labeled
// with the JSON-RPC method the call issues.
type meteredClient struct {
	eth     ETHClient
	chainID string
	metrics *instrumentation
}

var _ ETHClient = (*meteredClient)(nil)

func newMeteredClient(chainID string, eth ETHClient) *meteredClient {
	return &meteredClient{eth: eth, chainID: chainID, metrics: metrics}
}

// record labels the call. ethereum.NotFound is a valid answer, not a failure.
func (c *meteredClient) record(ctx context.Context, operation string, started time.Time, err error) {
	if errors.Is(err, ethereum.NotFound) {
		err = nil
	}

	otel.RecordOperation(ctx, c.metrics.Operation, operation, started,
		otel.AttrChainID.String(c.chainID),
		otel.AttrResultError(err),
		otel.AttrCode.String(errorCode(err)),
	)
}

// errorCode extracts the HTTP status or JSON-RPC error code go-ethereum
// attaches to a failed call; failures without one are "other".
func errorCode(err error) string {
	var (
		httpErr rpc.HTTPError
		rpcErr  rpc.Error
	)

	switch {
	case err == nil:
		return ""
	case errors.As(err, &httpErr):
		return "http_" + strconv.Itoa(httpErr.StatusCode)
	case errors.As(err, &rpcErr):
		return "jsonrpc_" + strconv.Itoa(rpcErr.ErrorCode())
	default:
		return "other"
	}
}

func (c *meteredClient) CodeAt(ctx context.Context, contract common.Address, blockNumber *big.Int) ([]byte, error) {
	started := time.Now()
	code, err := c.eth.CodeAt(ctx, contract, blockNumber)
	c.record(ctx, "eth_getCode", started, err)

	return code, err
}

func (c *meteredClient) CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	started := time.Now()
	output, err := c.eth.CallContract(ctx, call, blockNumber)
	c.record(ctx, "eth_call", started, err)

	return output, err
}

func (c *meteredClient) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	started := time.Now()
	header, err := c.eth.HeaderByNumber(ctx, number)
	c.record(ctx, "eth_getBlockByNumber", started, err)

	return header, err
}

func (c *meteredClient) GetProof(
	ctx context.Context,
	account common.Address,
	keys []string,
	blockNumber *big.Int,
) (*gethclient.AccountResult, error) {
	started := time.Now()
	proof, err := c.eth.GetProof(ctx, account, keys, blockNumber)
	c.record(ctx, "eth_getProof", started, err)

	return proof, err
}

func (c *meteredClient) PendingCodeAt(ctx context.Context, account common.Address) ([]byte, error) {
	started := time.Now()
	code, err := c.eth.PendingCodeAt(ctx, account)
	c.record(ctx, "eth_getCode", started, err)

	return code, err
}

func (c *meteredClient) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	started := time.Now()
	nonce, err := c.eth.PendingNonceAt(ctx, account)
	c.record(ctx, "eth_getTransactionCount", started, err)

	return nonce, err
}

func (c *meteredClient) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	started := time.Now()
	price, err := c.eth.SuggestGasPrice(ctx)
	c.record(ctx, "eth_gasPrice", started, err)

	return price, err
}

func (c *meteredClient) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {
	started := time.Now()
	tip, err := c.eth.SuggestGasTipCap(ctx)
	c.record(ctx, "eth_maxPriorityFeePerGas", started, err)

	return tip, err
}

func (c *meteredClient) EstimateGas(ctx context.Context, call ethereum.CallMsg) (uint64, error) {
	started := time.Now()
	gas, err := c.eth.EstimateGas(ctx, call)
	c.record(ctx, "eth_estimateGas", started, err)

	return gas, err
}

func (c *meteredClient) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	started := time.Now()
	err := c.eth.SendTransaction(ctx, tx)
	c.record(ctx, "eth_sendRawTransaction", started, err)

	return err
}

func (c *meteredClient) FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error) {
	started := time.Now()
	logs, err := c.eth.FilterLogs(ctx, query)
	c.record(ctx, "eth_getLogs", started, err)

	return logs, err
}

func (c *meteredClient) SubscribeFilterLogs(
	ctx context.Context,
	query ethereum.FilterQuery,
	ch chan<- types.Log,
) (ethereum.Subscription, error) {
	started := time.Now()
	sub, err := c.eth.SubscribeFilterLogs(ctx, query, ch)
	c.record(ctx, "eth_subscribe", started, err)

	return sub, err
}

func (c *meteredClient) BalanceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error) {
	started := time.Now()
	balance, err := c.eth.BalanceAt(ctx, account, blockNumber)
	c.record(ctx, "eth_getBalance", started, err)

	return balance, err
}

func (c *meteredClient) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	started := time.Now()
	receipt, err := c.eth.TransactionReceipt(ctx, txHash)
	c.record(ctx, "eth_getTransactionReceipt", started, err)

	return receipt, err
}

func (c *meteredClient) TransactionByHash(ctx context.Context, hash common.Hash) (*types.Transaction, bool, error) {
	started := time.Now()
	tx, pending, err := c.eth.TransactionByHash(ctx, hash)
	c.record(ctx, "eth_getTransactionByHash", started, err)

	return tx, pending, err
}
