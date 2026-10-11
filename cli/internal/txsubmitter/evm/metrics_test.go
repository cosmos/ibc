// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/cosmos/ibc/cli/internal/tests/mocks"
)

func TestEVMGasSpent(t *testing.T) {
	t.Run("accumulatesExactWeiAndSeparatesAttributes", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		instruments, reader := installTestMetrics(t)
		hash1 := common.HexToHash("0x01").String()
		hash2 := common.HexToHash("0x02").String()
		hash3 := common.HexToHash("0x03").String()
		hash4 := common.HexToHash("0x04").String()
		price1 := big.NewInt(2_000_000_000)
		price2 := new(big.Int).Exp(big.NewInt(10), big.NewInt(30), nil)
		expectedWalletA := new(big.Int).Add(
			new(big.Int).Mul(big.NewInt(21_000), price1),
			new(big.Int).Mul(big.NewInt(7), price2),
		)

		// ACT #1
		client := mocks.NewMockTxSubmitterETHClient(t)
		client.EXPECT().BalanceAt(mock.Anything, mock.Anything, (*big.Int)(nil)).
			Return(big.NewInt(0), nil).Times(3)
		instruments.setWallet("chain-a", "wallet-a", client)
		instruments.setWallet("chain-a", "wallet-b", client)
		instruments.setWallet("chain-b", "wallet-a", client)

		instruments.startTx("chain-a", "wallet-a", hash1)
		instruments.endTx("chain-a", successfulReceipt(t, hash1, 21_000, price1))
		instruments.startTx("chain-a", "wallet-a", hash2)
		instruments.endTx("chain-a", successfulReceipt(t, hash2, 7, price2))
		instruments.endTx("chain-a", successfulReceipt(t, hash2, 7, price2))
		instruments.startTx("chain-a", "wallet-b", hash3)
		instruments.endTx("chain-a", successfulReceipt(t, hash3, 10, weiPerNativeToken))
		instruments.startTx("chain-b", "wallet-a", hash4)
		instruments.endTx("chain-b", successfulReceipt(t, hash4, 20, weiPerNativeToken))

		// ASSERT #1
		assert.Equal(t, expectedWalletA, gasTotalWei(t, instruments, "chain-a", "wallet-a"))
		assert.Equal(
			t,
			new(big.Int).Mul(big.NewInt(10), weiPerNativeToken),
			gasTotalWei(t, instruments, "chain-a", "wallet-b"),
		)
		assert.Equal(
			t,
			new(big.Int).Mul(big.NewInt(20), weiPerNativeToken),
			gasTotalWei(t, instruments, "chain-b", "wallet-a"),
		)

		// ACT #2
		first := evmGasSpentValues(t, collectEVMGasSpent(ctx, t, reader))
		second := evmGasSpentValues(t, collectEVMGasSpent(ctx, t, reader))

		// ASSERT #2
		expectedValue, _ := new(big.Rat).SetFrac(expectedWalletA, weiPerNativeToken).Float64()
		expected := map[chainWallet]float64{
			{chainID: "chain-a", wallet: "wallet-a"}: expectedValue,
			{chainID: "chain-a", wallet: "wallet-b"}: 10,
			{chainID: "chain-b", wallet: "wallet-a"}: 20,
		}
		assert.Equal(t, expected, first)
		assert.Equal(t, expected, second)
	})

	for _, tt := range []struct {
		name        string
		owned       bool
		receipt     *types.Receipt
		expectedWei int64
	}{
		{
			name:        "ignoresUnownedReceipt",
			receipt:     successfulReceipt(t, "0x01", 21_000, big.NewInt(2_000_000_000)),
			expectedWei: 0,
		},
		{
			name:  "countsFailedReceipt",
			owned: true,
			receipt: &types.Receipt{
				TxHash:            common.HexToHash("0x02"),
				Status:            types.ReceiptStatusFailed,
				GasUsed:           21_000,
				EffectiveGasPrice: big.NewInt(2_000_000_000),
			},
			expectedWei: 42_000_000_000_000,
		},
		{
			name:  "clearsNilEffectiveGasPrice",
			owned: true,
			receipt: &types.Receipt{
				TxHash:  common.HexToHash("0x03"),
				Status:  types.ReceiptStatusSuccessful,
				GasUsed: 21_000,
			},
			expectedWei: 0,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			instruments, _ := installTestMetrics(t)
			txHash := tt.receipt.TxHash.String()
			instruments.setWallet(chainIDEth, "wallet-a", mocks.NewMockTxSubmitterETHClient(t))
			expected := big.NewInt(tt.expectedWei)
			if tt.owned {
				instruments.startTx(chainIDEth, "wallet-a", txHash)
			}

			// ACT #1
			instruments.endTx(chainIDEth, tt.receipt)

			// ASSERT #1
			assert.Equal(t, expected, gasTotalWei(t, instruments, chainIDEth, "wallet-a"))
			assert.Empty(t, instruments.txOwners)

			// ACT #2
			instruments.endTx(chainIDEth, successfulReceipt(t, txHash, 21_000, big.NewInt(2_000_000_000)))

			// ASSERT #2
			assert.Equal(t, expected, gasTotalWei(t, instruments, chainIDEth, "wallet-a"))
			assert.Empty(t, instruments.txOwners)
		})
	}
}

func TestForgetTx(t *testing.T) {
	instruments, _ := installTestMetrics(t)
	mined, pending := common.HexToHash("0x01"), common.HexToHash("0x02")
	instruments.startTx("1", "wallet", mined.Hex())
	instruments.startTx("1", "wallet", pending.Hex())
	instruments.forgetTx("1", mined.Hex())
	require.Len(t, instruments.txOwners, 1)
	key := transactionKey{chainID: "1", txHash: pending.Hex()}
	require.Contains(t, instruments.txOwners, key)
	instruments.forgetTx("1", pending.Hex())
	require.Empty(t, instruments.txOwners)
}

func TestGasBalance(t *testing.T) {
	t.Run("queriesEachWalletAndReemitsCache", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		instruments, reader := installTestMetrics(t)
		clientA := mocks.NewMockTxSubmitterETHClient(t)
		clientB := mocks.NewMockTxSubmitterETHClient(t)
		replacement := mocks.NewMockTxSubmitterETHClient(t)
		walletA := common.HexToAddress("0x01").String()
		walletB := common.HexToAddress("0x02").String()
		expectBalance(t, clientA, walletA, new(big.Int).Mul(big.NewInt(15), weiPerNativeToken))
		expectBalance(t, clientB, walletB, new(big.Int).Div(weiPerNativeToken, big.NewInt(2)))
		instruments.setWallet("chain-a", walletA, clientA)
		instruments.setWallet("chain-b", walletB, clientB)
		instruments.setWallet("chain-a", walletA, replacement)
		require.Len(t, instruments.wallets, 2)

		expected := map[chainWallet]float64{
			{chainID: "chain-a", wallet: walletA}: 15,
			{chainID: "chain-b", wallet: walletB}: 0.5,
		}

		// ACT #1
		first := gasBalanceValues(t, collectGasBalance(ctx, t, reader))

		// ASSERT #1
		assert.Equal(t, expected, first)

		// ACT #2
		second := gasBalanceValues(t, collectGasBalance(ctx, t, reader))

		// ASSERT #2
		assert.Equal(t, expected, second)
	})

	t.Run("refreshesAfterThreshold", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		instruments, reader := installTestMetrics(t)
		client := mocks.NewMockTxSubmitterETHClient(t)
		wallet := common.HexToAddress("0x01").String()
		expectBalance(t, client, wallet, new(big.Int).Mul(big.NewInt(15), weiPerNativeToken))
		instruments.setWallet("chain-a", wallet, client)

		// ACT #1
		first := gasBalanceValues(t, collectGasBalance(ctx, t, reader))

		// ASSERT #1
		assert.Equal(t, map[chainWallet]float64{
			{chainID: "chain-a", wallet: wallet}: 15,
		}, first)

		// ARRANGE #2
		expectBalance(t, client, wallet, new(big.Int).Mul(big.NewInt(20), weiPerNativeToken))
		expireBalanceCache(t, instruments)

		// ACT #2
		second := gasBalanceValues(t, collectGasBalance(ctx, t, reader))

		// ASSERT #2
		assert.Equal(t, map[chainWallet]float64{
			{chainID: "chain-a", wallet: wallet}: 20,
		}, second)
	})

	t.Run("reportsNegativeOneOnQueryError", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		instruments, reader := installTestMetrics(t)
		client := mocks.NewMockTxSubmitterETHClient(t)
		wallet := common.HexToAddress("0x01").String()
		client.EXPECT().BalanceAt(mock.Anything, common.HexToAddress(wallet), (*big.Int)(nil)).
			Return(nil, errors.New("rpc down")).Once()
		instruments.setWallet("chain-a", wallet, client)

		expected := map[chainWallet]float64{
			{chainID: "chain-a", wallet: wallet}: -1,
		}

		// ACT & ASSERT
		assert.Equal(t, expected, gasBalanceValues(t, collectGasBalance(ctx, t, reader)))
		assert.Equal(t, expected, gasBalanceValues(t, collectGasBalance(ctx, t, reader)))
	})

	t.Run("doesNotHoldInstrumentationLockDuringQuery", func(t *testing.T) {
		// ARRANGE
		instruments, reader := installTestMetrics(t)
		client := mocks.NewMockTxSubmitterETHClient(t)
		wallet := common.HexToAddress("0x01").String()
		started := make(chan struct{})
		release := make(chan struct{})
		client.EXPECT().BalanceAt(mock.Anything, common.HexToAddress(wallet), (*big.Int)(nil)).
			Run(func(context.Context, common.Address, *big.Int) {
				close(started)
				<-release
			}).
			Return(big.NewInt(0), nil).Once()
		instruments.setWallet("chain-a", wallet, client)

		collectDone := make(chan error, 1)
		go func() {
			var data metricdata.ResourceMetrics
			collectDone <- reader.Collect(context.Background(), &data)
		}()
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("balance query did not start")
		}

		// ACT
		acquired := instruments.mu.TryLock()
		if acquired {
			instruments.mu.Unlock()
		}
		close(release)

		// ASSERT
		assert.True(t, acquired)
		require.NoError(t, <-collectDone)
	})
}

func installTestMetrics(t *testing.T) (*instrumentation, *sdkmetric.ManualReader) {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	instruments, err := newInstrumentation(provider.Meter("test"))
	require.NoError(t, err)

	original := metrics
	metrics = instruments
	t.Cleanup(func() {
		metrics = original
		require.NoError(t, provider.Shutdown(context.Background()))
	})

	return instruments, reader
}

func collectEVMGasSpent(
	ctx context.Context,
	t *testing.T,
	reader *sdkmetric.ManualReader,
) metricdata.Sum[float64] {
	t.Helper()

	var data metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(ctx, &data))

	for _, scope := range data.ScopeMetrics {
		for _, candidate := range scope.Metrics {
			if candidate.Name == "evm_gas_spent" {
				sum, ok := candidate.Data.(metricdata.Sum[float64])
				require.True(t, ok)
				return sum
			}
		}
	}

	return metricdata.Sum[float64]{}
}

func collectGasBalance(
	ctx context.Context,
	t *testing.T,
	reader *sdkmetric.ManualReader,
) metricdata.Gauge[float64] {
	t.Helper()

	var data metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(ctx, &data))

	for _, scope := range data.ScopeMetrics {
		for _, candidate := range scope.Metrics {
			if candidate.Name == "evm_gas_balance" {
				gauge, ok := candidate.Data.(metricdata.Gauge[float64])
				require.True(t, ok)
				return gauge
			}
		}
	}

	return metricdata.Gauge[float64]{}
}

func successfulReceipt(t *testing.T, txHash string, gasUsed uint64, gasPrice *big.Int) *types.Receipt {
	t.Helper()

	return &types.Receipt{
		TxHash:            common.HexToHash(txHash),
		Status:            types.ReceiptStatusSuccessful,
		GasUsed:           gasUsed,
		EffectiveGasPrice: gasPrice,
	}
}

func gasTotalWei(t *testing.T, instruments *instrumentation, chainID, wallet string) *big.Int {
	t.Helper()

	instruments.mu.Lock()
	defer instruments.mu.Unlock()

	w, ok := instruments.wallets[chainWallet{chainID: chainID, wallet: wallet}]
	if !ok || w.cumulativeGasCostWei == nil {
		return nil
	}

	return new(big.Int).Set(w.cumulativeGasCostWei)
}

func expectBalance(t *testing.T, client *mocks.MockTxSubmitterETHClient, wallet string, wei *big.Int) {
	t.Helper()

	client.EXPECT().BalanceAt(mock.Anything, common.HexToAddress(wallet), (*big.Int)(nil)).
		Return(wei, nil).Once()
}

func expireBalanceCache(t *testing.T, instruments *instrumentation) {
	t.Helper()

	stale := time.Now().Add(-observationThreshold)

	for _, wallet := range instruments.snapshotWallets() {
		wallet.balanceMu.Lock()
		wallet.lastBalanceAt = stale
		wallet.balanceMu.Unlock()
	}
}

func gasBalanceValues(t *testing.T, gauge metricdata.Gauge[float64]) map[chainWallet]float64 {
	t.Helper()

	values := make(map[chainWallet]float64, len(gauge.DataPoints))
	for _, point := range gauge.DataPoints {
		chainID, _ := point.Attributes.Value(attribute.Key("chain_id"))
		wallet, _ := point.Attributes.Value(attribute.Key("wallet"))
		values[chainWallet{chainID: chainID.AsString(), wallet: wallet.AsString()}] = point.Value
	}

	return values
}

func evmGasSpentValues(t *testing.T, sum metricdata.Sum[float64]) map[chainWallet]float64 {
	t.Helper()

	values := make(map[chainWallet]float64, len(sum.DataPoints))
	for _, point := range sum.DataPoints {
		chainID, _ := point.Attributes.Value(attribute.Key("chain_id"))
		wallet, _ := point.Attributes.Value(attribute.Key("wallet"))
		values[chainWallet{chainID: chainID.AsString(), wallet: wallet.AsString()}] = point.Value
	}

	return values
}
