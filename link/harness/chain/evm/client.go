package evm

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/cosmos/ibc/link/harness/internal/poll"

	ethereum "github.com/ethereum/go-ethereum"
)

const (
	defaultGasLimit   = 21_000
	gasPaddingPercent = 20
	gasPaddingFixed   = 10_000
	// Anvil may suggest no priority fee on an empty mempool.
	fallbackTipCapWei = 1_000_000_000

	receiptPollInterval = 100 * time.Millisecond
	ReceiptTimeout      = 30 * time.Second
)

type EVMClient struct {
	client  *ethclient.Client
	chainID *big.Int

	// Local sends must be serialized across nonce reads and submission,
	// regardless of which account pays for the transaction.
	txMu sync.Mutex
}

// The caller remains responsible for readiness.
func NewConnectedClient(ctx context.Context, c *ethclient.Client) (*EVMClient, error) {
	id, err := c.ChainID(ctx)
	if err != nil {
		return nil, fmt.Errorf("query chain id: %w", err)
	}
	return &EVMClient{client: c, chainID: id}, nil
}

// It closes c on failure.
func NewVerifiedClient(
	ctx context.Context,
	c *ethclient.Client,
	wantChainID uint64,
	label string,
) (*EVMClient, error) {
	ec, err := NewConnectedClient(ctx, c)
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("connect %s: %w", label, err)
	}
	if got := ec.ChainID().Uint64(); got != wantChainID {
		ec.Close()
		return nil, fmt.Errorf("%s reports chain id %d, want %d", label, got, wantChainID)
	}
	return ec, nil
}

func (e *EVMClient) Client() *ethclient.Client { return e.client }

func (e *EVMClient) EVM() *EVMClient { return e }

func (e *EVMClient) ChainID() *big.Int { return new(big.Int).Set(e.chainID) }

func (e *EVMClient) RPCClient() *rpc.Client { return e.client.Client() }

func (e *EVMClient) Close() { e.client.Close() }

func (e *EVMClient) Height(ctx context.Context) (uint64, error) {
	return e.client.BlockNumber(ctx)
}

func Tail(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	return s[len(s)-maxBytes:]
}

func (e *EVMClient) Logs(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
	return e.client.FilterLogs(ctx, q)
}

// RequireEOA rejects addresses that cannot be funded consistently through a
// plain value transfer across managed EVM providers.
func (e *EVMClient) RequireEOA(ctx context.Context, address common.Address) error {
	if address == (common.Address{}) {
		return fmt.Errorf("EOA address is zero")
	}
	code, err := e.client.CodeAt(ctx, address, nil)
	if err != nil {
		return fmt.Errorf("query code at %s: %w", address, err)
	}
	if len(code) != 0 {
		return fmt.Errorf("address %s has contract code and is not an EOA", address)
	}
	return nil
}

// It returns only after mining and treats reverts as errors.
func (e *EVMClient) BroadcastTx(
	ctx context.Context,
	from Account,
	to *common.Address,
	data []byte,
	value *big.Int,
) (*types.Receipt, error) {
	signed, err := e.signAndSend(ctx, from, to, data, value)
	if err != nil {
		return nil, err
	}
	receipt, err := e.WaitForReceipt(ctx, signed.Hash())
	if err != nil {
		return nil, err
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return nil, fmt.Errorf("tx %s reverted on-chain (status %d)", signed.Hash(), receipt.Status)
	}
	return receipt, nil
}

func (e *EVMClient) signAndSend(
	ctx context.Context,
	from Account,
	to *common.Address,
	data []byte,
	value *big.Int,
) (*types.Transaction, error) {
	e.txMu.Lock()
	defer e.txMu.Unlock()

	signed, err := e.buildSignedTx(ctx, from, to, data, value)
	if err != nil {
		return nil, err
	}
	if err := e.client.SendTransaction(ctx, signed); err != nil {
		return nil, fmt.Errorf("send tx: %w", err)
	}
	return signed, nil
}

func (e *EVMClient) buildSignedTx(
	ctx context.Context,
	from Account,
	to *common.Address,
	data []byte,
	value *big.Int,
) (*types.Transaction, error) {
	nonce, err := e.client.PendingNonceAt(ctx, from.addr)
	if err != nil {
		return nil, fmt.Errorf("pending nonce: %w", err)
	}

	val := new(big.Int)
	if value != nil {
		val.Set(value)
	}

	est, err := e.client.EstimateGas(ctx, ethereum.CallMsg{From: from.addr, To: to, Value: val, Data: data})
	if err != nil {
		return nil, fmt.Errorf("estimate gas: %w", err)
	}
	gas := est + est*gasPaddingPercent/100 + gasPaddingFixed
	if gas < defaultGasLimit {
		gas = defaultGasLimit
	}

	tipCap, err := e.client.SuggestGasTipCap(ctx)
	if err != nil {
		return nil, fmt.Errorf("suggest gas tip cap: %w", err)
	}
	if tipCap == nil || tipCap.Sign() <= 0 {
		tipCap = big.NewInt(fallbackTipCapWei)
	}

	hdr, err := e.client.HeaderByNumber(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("latest header: %w", err)
	}
	feeCap := new(big.Int).Set(tipCap)
	if hdr.BaseFee != nil && hdr.BaseFee.Sign() > 0 {
		feeCap.Add(new(big.Int).Mul(hdr.BaseFee, big.NewInt(2)), tipCap)
	}

	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID:   e.chainID,
		Nonce:     nonce,
		To:        to,
		Value:     val,
		Gas:       gas,
		GasFeeCap: feeCap,
		GasTipCap: tipCap,
		Data:      data,
	})
	return signTx(tx, from.key, e.chainID)
}

func (e *EVMClient) WaitNextPendingTx(ctx context.Context) error {
	before, err := e.client.PendingTransactionCount(ctx)
	if err != nil {
		return err
	}
	if err := poll.Until(ctx, receiptPollInterval, ReceiptTimeout, func(ctx context.Context) (bool, error) {
		n, err := e.client.PendingTransactionCount(ctx)
		if err != nil {
			return false, err
		}
		return n > before, nil
	}); err != nil {
		return fmt.Errorf("waiting for pending tx count to exceed %d: %w", before, err)
	}
	return nil
}

func (e *EVMClient) WaitForReceipt(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
	var receipt *types.Receipt
	err := poll.Until(ctx, receiptPollInterval, ReceiptTimeout, func(ctx context.Context) (bool, error) {
		r, err := e.client.TransactionReceipt(ctx, hash)
		if err == nil && r != nil {
			receipt = r
			return true, nil
		}
		if err != nil && !errors.Is(err, ethereum.NotFound) {
			return false, err
		}
		return false, nil
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return nil, fmt.Errorf("receipt for %s not found in time: %w", hash, err)
		}
		return nil, fmt.Errorf("poll receipt %s: %w", hash, err)
	}
	return receipt, nil
}
