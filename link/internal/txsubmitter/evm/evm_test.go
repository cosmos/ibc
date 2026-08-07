package evm

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	ethereum "github.com/ethereum/go-ethereum"

	"github.com/cosmos/ibc/link/internal/service/signer"
	"github.com/cosmos/ibc/link/internal/tests/mocks"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

const (
	chainIDEth = "1"
	toAddress  = "0xe20BccD900Fa1B48f46F5a483d9De063b07eDFCC"
)

func newTestTxSubmitter(
	t *testing.T,
	opts ChainOptions,
) (*TxSubmitter, *mocks.MockTxSubmitterETHClient, signer.Signer) {
	t.Helper()

	eth := mocks.NewMockTxSubmitterETHClient(t)

	chainSigner, err := signer.GenerateLocalSecp256k1Signer()
	require.NoError(t, err)

	txSubmitter, err := New(chainIDEth, eth, chainSigner, opts)
	require.NoError(t, err)

	return txSubmitter, eth, chainSigner
}

func TestSubmit(t *testing.T) {
	ctx := context.Background()

	t.Run("signsAndBroadcasts", func(t *testing.T) {
		feeCapMult := 2.0
		txSubmitter, eth, chainSigner := newTestTxSubmitter(t, ChainOptions{
			TxSubmissionDelay:   time.Millisecond,
			GasFeeCapMultiplier: &feeCapMult,
		})

		eth.EXPECT().HeaderByNumber(ctx, (*big.Int)(nil)).Return(&types.Header{BaseFee: big.NewInt(100)}, nil).Once()
		eth.EXPECT().SuggestGasTipCap(ctx).Return(big.NewInt(10), nil).Once()
		eth.EXPECT().PendingCodeAt(ctx, mock.Anything).Return([]byte{0x60}, nil).Once()
		eth.EXPECT().EstimateGas(ctx, mock.Anything).Return(21000, nil).Once()
		eth.EXPECT().PendingNonceAt(ctx, mock.Anything).Return(7, nil).Once()

		var sent *types.Transaction
		eth.EXPECT().SendTransaction(ctx, mock.Anything).Run(func(_ context.Context, tx *types.Transaction) {
			sent = tx
		}).Return(nil).Once()

		sub, err := txSubmitter.Submit(ctx, v2.TxIntent{To: toAddress, Data: []byte{0xde, 0xad}})

		require.NoError(t, err)
		require.NotNil(t, sent)

		// nonce and gas parameters
		assert.Equal(t, uint64(7), sent.Nonce())
		assert.Equal(t, uint64(21000), sent.Gas())
		assert.Equal(t, big.NewInt(10), sent.GasTipCap())
		// feeCap = (tip + 2*baseFee) * multiplier = (10 + 200) * 2
		assert.Equal(t, big.NewInt(420), sent.GasFeeCap())

		// the signature recovers to the signer's address
		pub, err := crypto.DecompressPubkey(chainSigner.PublicKey())
		require.NoError(t, err)
		expectedFrom := crypto.PubkeyToAddress(*pub)

		from, err := types.Sender(types.LatestSignerForChainID(big.NewInt(1)), sent)
		require.NoError(t, err)
		assert.Equal(t, expectedFrom, from)
		assert.Equal(t, expectedFrom.String(), sub.RelayerAddress)
		assert.Equal(t, sent.Hash().String(), sub.TxHash)
	})

	t.Run("rejectsAddressWithoutCode", func(t *testing.T) {
		txSubmitter, eth, _ := newTestTxSubmitter(t, ChainOptions{TxSubmissionDelay: time.Millisecond})

		eth.EXPECT().HeaderByNumber(ctx, (*big.Int)(nil)).Return(&types.Header{BaseFee: big.NewInt(100)}, nil).Once()
		eth.EXPECT().SuggestGasTipCap(ctx).Return(big.NewInt(10), nil).Once()
		eth.EXPECT().PendingCodeAt(ctx, mock.Anything).Return(nil, nil).Once()

		_, err := txSubmitter.Submit(ctx, v2.TxIntent{To: toAddress, Data: []byte{0x01}})

		require.ErrorContains(t, err, "no contract code")
	})

	t.Run("rejectsChainWithoutBaseFee", func(t *testing.T) {
		txSubmitter, eth, _ := newTestTxSubmitter(t, ChainOptions{TxSubmissionDelay: time.Millisecond})

		eth.EXPECT().HeaderByNumber(ctx, (*big.Int)(nil)).Return(&types.Header{BaseFee: nil}, nil).Once()

		_, err := txSubmitter.Submit(ctx, v2.TxIntent{To: toAddress, Data: []byte{0x01}})

		require.ErrorContains(t, err, "EIP-1559")
	})
}

func TestShouldRetry(t *testing.T) {
	ctx := context.Background()
	txHash := "0x60016c34c02278856c81a41ce857ac4bb837a2f4a13c95207e08cbc9e8f2b706"

	t.Run("pendingNotExpired", func(t *testing.T) {
		txSubmitter, eth, _ := newTestTxSubmitter(t, ChainOptions{})
		eth.EXPECT().TransactionReceipt(ctx, mock.Anything).Return(nil, ethereum.NotFound).Once()
		eth.EXPECT().
			HeaderByNumber(ctx, (*big.Int)(nil)).
			Return(&types.Header{Time: uint64(time.Now().Unix())}, nil).
			Once()

		retry, err := txSubmitter.ShouldRetry(ctx, txHash, time.Now())

		require.ErrorIs(t, err, v2.ErrTxNotFound)
		assert.False(t, retry)
	})

	t.Run("pendingExpired", func(t *testing.T) {
		txSubmitter, eth, _ := newTestTxSubmitter(t, ChainOptions{})
		eth.EXPECT().TransactionReceipt(ctx, mock.Anything).Return(nil, ethereum.NotFound).Once()
		eth.EXPECT().
			HeaderByNumber(ctx, (*big.Int)(nil)).
			Return(&types.Header{Time: uint64(time.Now().Unix())}, nil).
			Once()

		retry, err := txSubmitter.ShouldRetry(ctx, txHash, time.Now().Add(-time.Hour))

		require.NoError(t, err)
		assert.True(t, retry)
	})

	t.Run("reverted", func(t *testing.T) {
		txSubmitter, eth, _ := newTestTxSubmitter(t, ChainOptions{})
		eth.EXPECT().
			TransactionReceipt(ctx, mock.Anything).
			Return(&types.Receipt{Status: types.ReceiptStatusFailed}, nil).
			Once()

		retry, err := txSubmitter.ShouldRetry(ctx, txHash, time.Now())

		require.NoError(t, err)
		assert.True(t, retry)
	})

	t.Run("confirmed", func(t *testing.T) {
		txSubmitter, eth, _ := newTestTxSubmitter(t, ChainOptions{})
		eth.EXPECT().
			TransactionReceipt(ctx, mock.Anything).
			Return(&types.Receipt{Status: types.ReceiptStatusSuccessful}, nil).
			Once()

		retry, err := txSubmitter.ShouldRetry(ctx, txHash, time.Now())

		require.NoError(t, err)
		assert.False(t, retry)
	})

	t.Run("receiptError", func(t *testing.T) {
		txSubmitter, eth, _ := newTestTxSubmitter(t, ChainOptions{})
		eth.EXPECT().TransactionReceipt(ctx, mock.Anything).Return(nil, errors.New("rpc down")).Once()

		_, err := txSubmitter.ShouldRetry(ctx, txHash, time.Now())

		require.ErrorContains(t, err, "rpc down")
	})
}
