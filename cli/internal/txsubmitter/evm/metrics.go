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
	"golang.org/x/sync/errgroup"

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

type balanceClient interface {
	BalanceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error)
}

type instrumentation struct {
	// EVMGasSpent is the total amount of gas spent on EVM chains in token (wei/10^18).
	// Note that this value is set per {chain_id, wallet} pair via OTEL observer.
	// This helps to convert cumulativeGasCostWei's bigInt to float64 only when requested by telemetry.
	EVMGasSpent metric.Float64ObservableCounter
	GasBalance  metric.Float64ObservableGauge

	txOwners             map[transactionKey]string
	cumulativeGasCostWei map[chainWallet]*big.Int
	clients              map[chainWallet]balanceClient
	lastObservation      map[chainWallet]time.Time
	mu                   sync.RWMutex
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
		EVMGasSpent:          evmGasSpent,
		GasBalance:           gasBalance,
		txOwners:             make(map[transactionKey]string),
		cumulativeGasCostWei: make(map[chainWallet]*big.Int),
		clients:              make(map[chainWallet]balanceClient),
		lastObservation:      make(map[chainWallet]time.Time),
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

func (m *instrumentation) setClient(chainID, wallet string, client balanceClient) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := chainWallet{chainID: chainID, wallet: wallet}
	if _, exists := m.clients[key]; exists {
		return
	}

	m.clients[key] = client
}

func (m *instrumentation) startTx(chainID, wallet, txHash string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.txOwners[transactionKey{chainID: chainID, txHash: txHash}] = wallet
}

func (m *instrumentation) endTx(chainID string, receipt *types.Receipt) {
	m.mu.Lock()
	defer m.mu.Unlock()

	txHash := receipt.TxHash.String()
	wallet, owned := m.txOwners[transactionKey{chainID: chainID, txHash: txHash}]
	if !owned {
		return
	}

	delete(m.txOwners, transactionKey{chainID: chainID, txHash: txHash})

	if receipt.Status != types.ReceiptStatusSuccessful || receipt.EffectiveGasPrice == nil {
		return
	}

	key := chainWallet{chainID: chainID, wallet: wallet}
	total, ok := m.cumulativeGasCostWei[key]
	if !ok {
		total = new(big.Int)
		m.cumulativeGasCostWei[key] = total
	}

	cost := new(big.Int).Mul(new(big.Int).SetUint64(receipt.GasUsed), receipt.EffectiveGasPrice)
	total.Add(total, cost)
}

func (m *instrumentation) forgetTx(chainID, txHash string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.txOwners, transactionKey{chainID: chainID, txHash: txHash})
}

func (m *instrumentation) observeEVMGasSpent(_ context.Context, observer metric.Observer) error {
	type observation struct {
		key   chainWallet
		total *big.Int
	}

	m.mu.RLock()
	observations := make([]observation, 0, len(m.cumulativeGasCostWei))
	for key, total := range m.cumulativeGasCostWei {
		observations = append(observations, observation{key: key, total: new(big.Int).Set(total)})
	}
	m.mu.RUnlock()

	for _, observation := range observations {
		value, _ := new(big.Rat).SetFrac(observation.total, weiPerNativeToken).Float64()
		observer.ObserveFloat64(m.EVMGasSpent, value, metric.WithAttributes(
			otel.AttrChainID.String(observation.key.chainID),
			otel.AttrWallet.String(observation.key.wallet),
		))
	}

	return nil
}

func (m *instrumentation) observeGasBalance(ctx context.Context, observer metric.Observer) error {
	var (
		now     = time.Now()
		clients = make(map[chainWallet]balanceClient)
		eg      errgroup.Group
	)

	m.mu.Lock()
	for key, client := range m.clients {
		// To prevent live frequent RPC calls, skip if observed within the threshold.
		if now.Sub(m.lastObservation[key]) < observationThreshold {
			continue
		}

		m.lastObservation[key] = now
		clients[key] = client
	}
	m.mu.Unlock()

	for key, client := range clients {
		addr := common.HexToAddress(key.wallet)
		attr := metric.WithAttributes(
			otel.AttrChainID.String(key.chainID),
			otel.AttrWallet.String(key.wallet),
		)

		eg.Go(func() error {
			balance, err := client.BalanceAt(ctx, addr, nil)
			if err != nil {
				slog.Error("Metrics: failed to get balance", "chainID", key.chainID, "wallet", key.wallet, "error", err)
				observer.ObserveFloat64(m.GasBalance, -1, attr)
				return nil
			}

			value, _ := new(big.Rat).SetFrac(balance, weiPerNativeToken).Float64()
			observer.ObserveFloat64(m.GasBalance, value, attr)

			return nil
		})
	}

	return eg.Wait()
}
