// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/core/types"
	"go.opentelemetry.io/otel/metric"

	"github.com/cosmos/ibc/cli/internal/otel"
)

var weiPerNativeToken = new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)

type transactionKey struct {
	chainID string
	txHash  string
}

type gasCostKey struct {
	chainID string
	wallet  string
}

type instrumentation struct {
	// EVMGasSpent is the total amount of gas spent on EVM chains in token (wei/10^18).
	// Note that this value is set per {chain_id, wallet} pair via OTEL observer.
	// This helps to convert cumulativeGasCostWei's bigInt to float64 only when requested by telemetry.
	EVMGasSpent metric.Float64ObservableCounter

	txOwners             map[transactionKey]string
	cumulativeGasCostWei map[gasCostKey]*big.Int
	mu                   sync.Mutex
}

var metrics = otel.RegisterMetrics("relayer", newInstrumentation)

func newInstrumentation(m metric.Meter) (*instrumentation, error) {
	evmGasSpent, err := m.Float64ObservableCounter("evm_gas_spent")
	if err != nil {
		return nil, err
	}

	instruments := &instrumentation{
		EVMGasSpent:          evmGasSpent,
		txOwners:             make(map[transactionKey]string),
		cumulativeGasCostWei: make(map[gasCostKey]*big.Int),
	}

	_, err = m.RegisterCallback(instruments.observeEVMGasSpent, evmGasSpent)
	if err != nil {
		return nil, err
	}

	return instruments, nil
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

	key := gasCostKey{chainID: chainID, wallet: wallet}
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
		key   gasCostKey
		total *big.Int
	}

	m.mu.Lock()
	observations := make([]observation, 0, len(m.cumulativeGasCostWei))
	for key, total := range m.cumulativeGasCostWei {
		observations = append(observations, observation{key: key, total: new(big.Int).Set(total)})
	}
	m.mu.Unlock()

	for _, observation := range observations {
		value, _ := new(big.Rat).SetFrac(observation.total, weiPerNativeToken).Float64()
		observer.ObserveFloat64(m.EVMGasSpent, value, metric.WithAttributes(
			otel.AttrChainID.String(observation.key.chainID),
			otel.AttrWallet.String(observation.key.wallet),
		))
	}

	return nil
}
