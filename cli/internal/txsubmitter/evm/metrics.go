// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"log/slog"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"go.opentelemetry.io/otel/metric"

	"github.com/cosmos/ibc/cli/internal/otel"
)

var weiPerNativeToken = new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)

const observationThreshold = 10 * time.Second

type transactionKey struct {
	chainID string
	txHash  string
}

type chainWallet struct {
	chainID string
	wallet  string
}

type walletMetrics struct {
	address common.Address
	client  balanceClient

	lastBalanceAt  time.Time
	lastBalance    *big.Int
	lastBalanceErr error
	balanceMu      sync.Mutex

	cumulativeGasCostWei *big.Int
}

type balanceClient interface {
	BalanceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error)
}

type instrumentation struct {
	// EVMGasSpent is the total amount of gas spent on EVM chains in token (wei/10^18).
	// Note that this value is set per {chain_id, wallet} pair via OTEL observer.
	// This helps to convert cumulativeGasCostWei's bigInt to float64 only when requested by telemetry.
	EVMGasSpent metric.Float64ObservableCounter
	GasBalance  metric.Float64ObservableGauge

	txOwners map[transactionKey]chainWallet
	wallets  map[chainWallet]*walletMetrics
	mu       sync.RWMutex
}

var metrics = otel.RegisterMetrics("relayer", newInstrumentation)

func newInstrumentation(m metric.Meter) (*instrumentation, error) {
	evmGasSpent, err := m.Float64ObservableCounter("evm_gas_spent")
	if err != nil {
		return nil, err
	}

	gasBalance, err := m.Float64ObservableGauge("evm_gas_balance")
	if err != nil {
		return nil, err
	}

	instruments := &instrumentation{
		EVMGasSpent: evmGasSpent,
		GasBalance:  gasBalance,
		txOwners:    make(map[transactionKey]chainWallet),
		wallets:     make(map[chainWallet]*walletMetrics),
	}

	_, err = m.RegisterCallback(instruments.observeEVMGasSpent, evmGasSpent)
	if err != nil {
		return nil, err
	}

	_, err = m.RegisterCallback(instruments.observeGasBalance, gasBalance)
	if err != nil {
		return nil, err
	}

	return instruments, nil
}

func (m *instrumentation) setWallet(chainID, wallet string, client balanceClient) {
	key := chainWallet{chainID: chainID, wallet: wallet}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.wallets[key]; exists {
		return
	}

	m.wallets[key] = &walletMetrics{
		address:              common.HexToAddress(wallet),
		client:               client,
		lastBalanceAt:        time.Time{},
		lastBalance:          big.NewInt(0),
		cumulativeGasCostWei: big.NewInt(0),
	}
}

func (m *instrumentation) startTx(chainID, wallet, txHash string) {
	walletKey := chainWallet{chainID: chainID, wallet: wallet}
	txKey := transactionKey{chainID: chainID, txHash: txHash}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.txOwners[txKey] = walletKey
}

func (m *instrumentation) endTx(chainID string, receipt *types.Receipt) {
	txHash := receipt.TxHash.String()
	txKey := transactionKey{chainID: chainID, txHash: txHash}

	m.mu.Lock()
	defer m.mu.Unlock()

	walletKey, owned := m.txOwners[txKey]
	if !owned {
		return
	}

	delete(m.txOwners, transactionKey{chainID: chainID, txHash: txHash})

	if receipt.Status != types.ReceiptStatusSuccessful || receipt.EffectiveGasPrice == nil {
		return
	}

	w, ok := m.wallets[walletKey]
	if !ok {
		return
	}

	cost := new(big.Int).Mul(new(big.Int).SetUint64(receipt.GasUsed), receipt.EffectiveGasPrice)
	w.cumulativeGasCostWei.Add(w.cumulativeGasCostWei, cost)
}

func (m *instrumentation) forgetTx(chainID, txHash string) {
	txKey := transactionKey{chainID: chainID, txHash: txHash}

	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.txOwners, txKey)
}

func (m *instrumentation) observeEVMGasSpent(_ context.Context, observer metric.Observer) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for walletKey, w := range m.wallets {
		// float64(total / 10^18)
		gasSpent, _ := new(big.Rat).SetFrac(w.cumulativeGasCostWei, weiPerNativeToken).Float64()

		observer.ObserveFloat64(m.EVMGasSpent, gasSpent, metric.WithAttributes(
			otel.AttrChainID.String(walletKey.chainID),
			otel.AttrWallet.String(walletKey.wallet),
		))
	}

	return nil
}

func (m *instrumentation) observeGasBalance(ctx context.Context, observer metric.Observer) error {
	now := time.Now()
	wg := sync.WaitGroup{}

	wallets := m.snapshotWallets()

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	for key, w := range wallets {
		wg.Add(1)

		go func(key chainWallet, w *walletMetrics) {
			defer wg.Done()

			attrs := metric.WithAttributes(
				otel.AttrChainID.String(key.chainID),
				otel.AttrWallet.String(key.wallet),
			)

			balance, err := w.getBalance(ctx, now)
			if err != nil {
				slog.Error("Metrics: getBalance failed", "chainID", key.chainID, "wallet", key.wallet, "error", err)
				observer.ObserveFloat64(m.GasBalance, -1, attrs)
				return
			}

			value, _ := new(big.Rat).SetFrac(balance, weiPerNativeToken).Float64()
			observer.ObserveFloat64(m.GasBalance, value, attrs)
		}(key, w)
	}

	wg.Wait()

	return nil
}

func (m *instrumentation) snapshotWallets() map[chainWallet]*walletMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	wallets := make(map[chainWallet]*walletMetrics, len(m.wallets))
	for key, w := range m.wallets {
		wallets[key] = w
	}

	return wallets
}

func (w *walletMetrics) getBalance(ctx context.Context, now time.Time) (*big.Int, error) {
	w.balanceMu.Lock()
	defer w.balanceMu.Unlock()

	if now.Sub(w.lastBalanceAt) < observationThreshold {
		return w.lastBalance, w.lastBalanceErr
	}

	balance, err := w.client.BalanceAt(ctx, w.address, nil)

	w.lastBalance = balance
	w.lastBalanceErr = err
	w.lastBalanceAt = now

	return balance, err
}
