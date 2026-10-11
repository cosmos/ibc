// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besuqbft"
	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/internal/service/signer"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

const chainIDEth = "1"

// Submission is serialized by the submitter, including access to this RPC stub.
type testRPC struct {
	mined, pending uint64
	sent           []*types.Transaction
	send           func(*types.Transaction) error
	gasErr         error
	nonceErr       error
	receipt        *types.Receipt
	receiptErr     error
}

func (r *testRPC) BalanceAt(context.Context, common.Address, *big.Int) (*big.Int, error) {
	return big.NewInt(1), nil
}

func (r *testRPC) HeaderByNumber(context.Context, *big.Int) (*types.Header, error) {
	return &types.Header{BaseFee: big.NewInt(100)}, nil
}
func (r *testRPC) SuggestGasTipCap(context.Context) (*big.Int, error) { return big.NewInt(10), nil }
func (r *testRPC) PendingCodeAt(context.Context, common.Address) ([]byte, error) {
	return []byte{1}, nil
}

func (r *testRPC) EstimateGasAtBlock(_ context.Context, _ ethereum.CallMsg, block *big.Int) (uint64, error) {
	if block != nil {
		return 0, errors.New("expected mined state")
	}
	return 100_000, r.gasErr
}

func (r *testRPC) NonceAt(context.Context, common.Address, *big.Int) (uint64, error) {
	return r.mined, r.nonceErr
}

func (r *testRPC) PendingNonceAt(context.Context, common.Address) (uint64, error) {
	return r.pending, nil
}

func (r *testRPC) TransactionReceipt(context.Context, common.Hash) (*types.Receipt, error) {
	if r.receipt != nil || r.receiptErr != nil {
		return r.receipt, r.receiptErr
	}
	return nil, ethereum.NotFound
}

func (r *testRPC) SendTransaction(_ context.Context, tx *types.Transaction) error {
	r.sent = append(r.sent, tx)
	if r.send != nil {
		return r.send(tx)
	}
	r.pending = max(r.pending, tx.Nonce()+1)
	return nil
}

func newSubmitter(t *testing.T) (*TxSubmitter, *testRPC) {
	t.Helper()
	key, err := signer.GenerateLocalSecp256k1Signer()
	require.NoError(t, err)
	rpc := &testRPC{}
	c, err := New(chainIDEth, rpc, key, ChainOptions{})
	require.NoError(t, err)
	return c, rpc
}

func testIntent() v2.TxIntent {
	return v2.TxIntent{To: "0x1111111111111111111111111111111111111111", Data: []byte{1}}
}
func expireNonce(c *TxSubmitter) { c.pendingSince = time.Now().Add(-2 * c.opts.ResubmitAfter) }

func submit(t *testing.T, c *TxSubmitter) error {
	t.Helper()
	_, err := c.Submit(t.Context(), testIntent())
	return err
}

func TestConcurrentSubmissionsAllocateDistinctNonces(t *testing.T) {
	c, rpc := newSubmitter(t)
	const count = 16
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for range count {
		wg.Go(func() { errs <- submit(t, c) })
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	require.Len(t, rpc.sent, count)
	for i, tx := range rpc.sent {
		require.Equal(t, uint64(i), tx.Nonce())
	}
	require.ErrorIs(t, submit(t, c), ErrBusy)
}

func TestStalledNonceRecoveredAtCapacity(t *testing.T) {
	c, rpc := newSubmitter(t)
	c.opts.MaxInFlight = 2
	require.NoError(t, submit(t, c))
	first := rpc.sent[0]
	require.NoError(t, submit(t, c))
	require.ErrorIs(t, submit(t, c), ErrBusy)
	expireNonce(c)
	require.NoError(t, submit(t, c))
	replacement := rpc.sent[2]
	require.Equal(t, first.Nonce(), replacement.Nonce())
	require.Positive(t, replacement.GasFeeCap().Cmp(first.GasFeeCap()))
	require.Positive(t, replacement.GasTipCap().Cmp(first.GasTipCap()))
	require.ErrorIs(t, submit(t, c), ErrBusy, "give the replacement time to mine")
	rpc.mined = 2
	require.NoError(t, submit(t, c))
	require.Equal(t, uint64(2), rpc.sent[3].Nonce())
}

func TestRetryBumpsOnUnderpricedAndPreservesProgress(t *testing.T) {
	c, rpc := newSubmitter(t)
	rpc.pending = 1
	rpc.send = func(tx *types.Transaction) error {
		if tx.GasTipCap().Cmp(big.NewInt(100)) < 0 {
			return errors.New("replacement transaction underpriced")
		}
		return nil
	}
	// A fresh process gives the unknown transaction a grace period.
	_, err := c.selectNonce(t.Context(), time.Now())
	require.NoError(t, err)
	expireNonce(c)
	require.ErrorContains(t, submit(t, c), "still underpriced")
	require.Len(t, rpc.sent, maxBroadcastAttempts)
	// The next call continues escalating rather than restarting at the estimate.
	require.NoError(t, submit(t, c))
	require.Greater(t, len(rpc.sent), maxBroadcastAttempts)
	for i, tx := range rpc.sent {
		require.Zero(t, tx.Nonce())
		if i > 0 {
			require.Positive(t, tx.GasFeeCap().Cmp(rpc.sent[i-1].GasFeeCap()))
			require.Positive(t, tx.GasTipCap().Cmp(rpc.sent[i-1].GasTipCap()))
		}
	}
}

func TestAcceptedTransactionStillGetsHigherFeesAfterStalling(t *testing.T) {
	c, rpc := newSubmitter(t)
	for range 3 {
		require.NoError(t, submit(t, c))
		expireNonce(c)
	}
	require.Len(t, rpc.sent, 3)
	for i, tx := range rpc.sent {
		require.Zero(t, tx.Nonce())
		if i > 0 {
			require.Positive(t, tx.GasTipCap().Cmp(rpc.sent[i-1].GasTipCap()))
		}
	}
}

func TestLostResponseAndDroppedTransactionRecover(t *testing.T) {
	c, rpc := newSubmitter(t)
	rpc.send = func(*types.Transaction) error { return errors.New("connection closed after acceptance") }
	submission, err := c.Submit(t.Context(), testIntent())
	require.Error(t, err)
	require.Equal(t, rpc.sent[0].Hash().Hex(), submission.TxHash)
	require.Equal(t, c.address.Hex(), submission.RelayerAddress)
	require.WithinDuration(t, time.Now(), submission.SubmittedAt, time.Second)
	rpc.send = nil
	// The node no longer reports the original, but local allocation remembers the gap.
	expireNonce(c)
	require.NoError(t, submit(t, c))
	require.Zero(t, rpc.sent[1].Nonce())
}

func TestRestartRecoversUnknownGap(t *testing.T) {
	c, rpc := newSubmitter(t)
	rpc.mined, rpc.pending = 7, 9
	require.NoError(t, submit(t, c))
	require.Equal(t, uint64(9), rpc.sent[0].Nonce())
	restarted, err := New(chainIDEth, rpc, c.signer, c.opts)
	require.NoError(t, err)
	_, err = restarted.selectNonce(t.Context(), time.Now())
	require.NoError(t, err)
	expireNonce(restarted)
	require.NoError(t, submit(t, restarted))
	require.Equal(t, uint64(7), rpc.sent[1].Nonce())
}

func TestPreparationFailureDoesNotAllocateNonce(t *testing.T) {
	c, rpc := newSubmitter(t)
	rpc.gasErr = errors.New("execution reverted")
	submission, err := c.Submit(t.Context(), testIntent())
	require.ErrorIs(t, err, rpc.gasErr)
	require.Nil(t, submission)
	require.Empty(t, rpc.sent)
	require.Zero(t, c.nextNonce)
	rpc.gasErr = nil
	require.NoError(t, submit(t, c))
	require.Zero(t, rpc.sent[0].Nonce())
}

type stalledSigner struct{ signer.Signer }

func (s stalledSigner) Sign(ctx context.Context, _ []byte) ([]byte, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestSignerDeadlineDoesNotConsumeNonce(t *testing.T) {
	c, rpc := newSubmitter(t)
	key := c.signer
	c.signer = stalledSigner{key}
	c.opts.RequestTimeout = 10 * time.Millisecond
	require.ErrorIs(t, submit(t, c), context.DeadlineExceeded)
	require.Empty(t, rpc.sent)
	c.signer = key
	c.opts.RequestTimeout = time.Second
	require.NoError(t, submit(t, c))
	require.Zero(t, rpc.sent[0].Nonce())
}

func TestReplacementFeeCapBoundsRetries(t *testing.T) {
	c, rpc := newSubmitter(t)
	c.opts.MaxFeePerGas = big.NewInt(250)
	require.NoError(t, submit(t, c))
	expireNonce(c)
	require.ErrorContains(t, submit(t, c), "fee cap")
	require.Len(t, rpc.sent, 1)
}

func TestNonceQueryFailureDoesNotBroadcast(t *testing.T) {
	c, rpc := newSubmitter(t)
	rpc.nonceErr = errors.New("unavailable")
	require.ErrorIs(t, submit(t, c), rpc.nonceErr)
	require.Empty(t, rpc.sent)
}

func TestReorgAllowsLowestNonceReuse(t *testing.T) {
	c, rpc := newSubmitter(t)
	rpc.mined, rpc.pending = 7, 7
	require.NoError(t, submit(t, c))
	rpc.mined, rpc.pending = 8, 8
	require.NoError(t, submit(t, c))
	rpc.mined, rpc.pending = 7, 7
	_, err := c.selectNonce(t.Context(), time.Now())
	require.NoError(t, err)
	expireNonce(c)
	require.NoError(t, submit(t, c))
	require.Equal(t, uint64(7), rpc.sent[2].Nonce())
}

func TestDroppedNonceRecoversBelowLocalAllocation(t *testing.T) {
	c, rpc := newSubmitter(t)
	rpc.mined, rpc.pending = 7, 7
	for range 3 {
		require.NoError(t, submit(t, c))
	}
	require.Equal(t, uint64(10), c.nextNonce)
	rpc.pending = 7
	nonce, err := c.selectNonce(t.Context(), time.Now())
	require.NoError(t, err)
	require.Equal(t, uint64(10), nonce, "allow the existing transactions time to mine")
	expireNonce(c)
	require.NoError(t, submit(t, c))
	require.Equal(t, uint64(7), rpc.sent[3].Nonce())
}

func TestShouldRetry(t *testing.T) {
	for _, tt := range []struct {
		name    string
		receipt *types.Receipt
		err     error
		expired bool
		retry   bool
		wantErr error
	}{
		{name: "pending", wantErr: v2.ErrTxNotFound},
		{name: "expired missing receipt", expired: true, retry: true},
		{name: "mined success", receipt: &types.Receipt{Status: types.ReceiptStatusSuccessful}},
		{name: "old mined success", receipt: &types.Receipt{Status: types.ReceiptStatusSuccessful}, expired: true},
		{name: "mined failure", receipt: &types.Receipt{Status: types.ReceiptStatusFailed}, retry: true},
		{name: "RPC unavailable", err: context.DeadlineExceeded, expired: true, wantErr: context.DeadlineExceeded},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, rpc := newSubmitter(t)
			rpc.receipt, rpc.receiptErr = tt.receipt, tt.err
			sentAt := time.Now()
			if tt.expired {
				sentAt = sentAt.Add(-2 * c.opts.ResubmitAfter)
			}
			retry, err := c.ShouldRetry(t.Context(), common.Hash{}.Hex(), sentAt)
			require.ErrorIs(t, err, tt.wantErr)
			require.Equal(t, tt.retry, retry)
		})
	}
}

func TestSubmitRejectsContractSimulationRevert(t *testing.T) {
	submitter, eth := newSubmitter(t)
	revert := revertError{
		code: -32000,
		data: revertData(
			t,
			besuqbft.ContractMetaData,
			"ConsensusStateExpired",
			uint64(100),
			big.NewInt(121),
			uint64(20),
		),
	}
	eth.gasErr = revert
	result, err := submitter.Submit(t.Context(), testIntent())
	require.Nil(t, result)
	require.ErrorIs(t, err, revert)
	require.EqualError(t, err, "estimating gas: ConsensusStateExpired(100, 121, 20): execution reverted")
	require.Empty(t, eth.sent)
	require.Zero(t, submitter.nextNonce)
}
