// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"math/big"
	"sync"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc-contracts/packages/go-abigen/besuqbft"
	"github.com/cosmos/ibc/cli/internal/service/signer"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

const chainIDEth = "1"

// Submission is serialized by the submitter, including access to this RPC stub.
type testRPC struct {
	height         uint64
	baseFee, tip   int64
	mined, pending uint64
	sent           []*types.Transaction
	send           func(*types.Transaction) error
	headerCalls    int
	tipCalls       int
	codeCalls      int
	gasCalls       int
	gas            uint64
	gasErr         error
	nonceErr       error
	receipt        *types.Receipt
	receiptErr     error
}

func (r *testRPC) BalanceAt(context.Context, common.Address, *big.Int) (*big.Int, error) {
	return big.NewInt(1), nil
}

func (r *testRPC) HeaderByNumber(context.Context, *big.Int) (*types.Header, error) {
	r.headerCalls++
	return &types.Header{Number: new(big.Int).SetUint64(r.height), BaseFee: big.NewInt(r.baseFee)}, nil
}

func (r *testRPC) SuggestGasTipCap(context.Context) (*big.Int, error) {
	r.tipCalls++
	return big.NewInt(r.tip), nil
}

func (r *testRPC) PendingCodeAt(context.Context, common.Address) ([]byte, error) {
	r.codeCalls++
	return []byte{1}, nil
}

func (r *testRPC) CallContract(context.Context, ethereum.CallMsg, *big.Int) ([]byte, error) {
	return nil, errors.New("unexpected call")
}

func (r *testRPC) EstimateGasAtBlock(_ context.Context, _ ethereum.CallMsg, block *big.Int) (uint64, error) {
	r.gasCalls++
	if block != nil {
		return 0, errors.New("expected mined state")
	}
	return r.gas, r.gasErr
}

func (r *testRPC) NonceAt(_ context.Context, _ common.Address, block *big.Int) (uint64, error) {
	if block == nil || block.Uint64() != r.height {
		return 0, errors.New("nonce query must be pinned to the observed block height")
	}
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
	rpc := &testRPC{height: 100, baseFee: 100, tip: 10, gas: 100_000}
	c, err := New(chainIDEth, rpc, key, ChainOptions{})
	require.NoError(t, err)
	return c, rpc
}

func testIntent() v2.TxIntent {
	return v2.TxIntent{To: "0x1111111111111111111111111111111111111111", Data: []byte{1}}
}

func expireNonce(c *TxSubmitter) {
	c.lowestUnminedNoncePendingSince = time.Now().Add(-2 * c.opts.PendingTxTimeout)
}

func submit(t *testing.T, c *TxSubmitter) error {
	t.Helper()
	submission, err := c.Submit(t.Context(), testIntent())
	if err != nil {
		require.Nil(t, submission)
	}
	return err
}

func TestConcurrentSubmissionsAllocateDistinctNonces(t *testing.T) {
	c, rpc := newSubmitter(t)
	const count = maxInFlight
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
	for range maxInFlight {
		require.NoError(t, submit(t, c))
	}
	first := rpc.sent[0]
	require.ErrorIs(t, submit(t, c), ErrBusy)
	expireNonce(c)
	require.NoError(t, submit(t, c))
	replacement := rpc.sent[maxInFlight]
	require.Equal(t, first.Nonce(), replacement.Nonce())
	require.Positive(t, replacement.GasFeeCap().Cmp(first.GasFeeCap()))
	require.Positive(t, replacement.GasTipCap().Cmp(first.GasTipCap()))
	require.ErrorIs(t, submit(t, c), ErrBusy, "give the replacement time to mine")
	rpc.mined = maxInFlight
	require.NoError(t, submit(t, c))
	require.Equal(t, uint64(maxInFlight), rpc.sent[maxInFlight+1].Nonce())
}

func TestRetryBumpsOnUnderpricedAndPreservesProgress(t *testing.T) {
	for _, message := range []string{
		"replacement transaction underpriced",
		"Replacement transaction underpriced",
		"rpc error: Replacement transaction underpriced",
	} {
		t.Run(message, func(t *testing.T) {
			c, rpc := newSubmitter(t)
			rpc.pending = 1
			rpc.send = func(tx *types.Transaction) error {
				if tx.GasTipCap().Cmp(big.NewInt(100)) < 0 {
					return errors.New(message)
				}
				return nil
			}
			// A fresh process gives the unknown transaction a grace period.
			_, _, err := c.selectNonce(t.Context(), time.Now())
			require.NoError(t, err)
			expireNonce(c)
			require.ErrorContains(t, submit(t, c), "still underpriced")
			require.Len(t, rpc.sent, maxBroadcastAttempts)
			require.Equal(t, 1, rpc.codeCalls)
			require.Equal(t, 1, rpc.gasCalls)
			require.Equal(t, 1, rpc.tipCalls)
			require.Equal(t, 2, rpc.headerCalls, "one initial observation and one submission")
			// The next call continues escalating rather than restarting at the estimate.
			rpc.gas = 150_000
			require.NoError(t, submit(t, c))
			require.Greater(t, len(rpc.sent), maxBroadcastAttempts)
			require.Equal(t, 2, rpc.codeCalls)
			require.Equal(t, 2, rpc.gasCalls)
			require.Equal(t, 2, rpc.tipCalls)
			require.Equal(t, 3, rpc.headerCalls)
			for i, tx := range rpc.sent {
				require.Zero(t, tx.Nonce())
				require.LessOrEqual(t, tx.GasFeeCap().Int64(), int64(840))
				if i < maxBroadcastAttempts {
					require.Equal(t, uint64(100_000), tx.Gas())
				} else {
					require.Equal(t, uint64(150_000), tx.Gas())
				}
				if i > 0 {
					require.GreaterOrEqual(t, tx.GasFeeCap().Cmp(rpc.sent[i-1].GasFeeCap()), 0)
					require.Positive(t, tx.GasTipCap().Cmp(rpc.sent[i-1].GasTipCap()))
				}
			}
		})
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

func TestBroadcastErrorDoesNotAdvanceNonceOrFees(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
	}{
		{name: "rejected", err: errors.New("transaction pool full")},
		{name: "insufficient funds", err: errors.New("insufficient funds for gas * price + value")},
		{name: "underpriced without replacement", err: errors.New("transaction underpriced")},
		{name: "lost response", err: context.DeadlineExceeded},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, rpc := newSubmitter(t)
			rpc.send = func(*types.Transaction) error { return tt.err }
			submission, err := c.Submit(t.Context(), testIntent())
			require.ErrorIs(t, err, tt.err)
			require.Nil(t, submission)
			require.Len(t, rpc.sent, 1)
			require.Zero(t, c.nextNonce)

			for range 2 {
				require.ErrorIs(t, submit(t, c), tt.err)
				require.Zero(t, c.nextNonce)
			}
			rpc.send = nil
			submission, err = c.Submit(t.Context(), testIntent())
			require.NoError(t, err)
			require.NotNil(t, submission)
			require.Len(t, rpc.sent, 4)
			require.Equal(t, rpc.sent[3].Hash().Hex(), submission.TxHash)
			require.Equal(t, c.address.Hex(), submission.RelayerAddress)
			require.Equal(t, testIntent().Data, rpc.sent[3].Data())
			require.WithinDuration(t, time.Now(), submission.SubmittedAt, time.Second)
			for _, tx := range rpc.sent {
				require.Zero(t, tx.Nonce())
				require.Equal(t, rpc.sent[0].GasFeeCap(), tx.GasFeeCap())
				require.Equal(t, rpc.sent[0].GasTipCap(), tx.GasTipCap())
			}
			require.Equal(t, uint64(1), c.nextNonce)
		})
	}
}

func TestRejectedReplacementPreservesAcceptedFees(t *testing.T) {
	c, rpc := newSubmitter(t)
	require.NoError(t, submit(t, c))
	original := rpc.sent[0]
	expireNonce(c)
	rejected := errors.New("transaction pool full")
	rpc.send = func(*types.Transaction) error { return rejected }
	for range 3 {
		require.ErrorIs(t, submit(t, c), rejected)
	}
	rpc.send = nil
	require.NoError(t, submit(t, c))
	require.Len(t, rpc.sent, 5)
	firstReplacement := rpc.sent[1]
	require.Positive(t, firstReplacement.GasFeeCap().Cmp(original.GasFeeCap()))
	require.Positive(t, firstReplacement.GasTipCap().Cmp(original.GasTipCap()))
	for _, tx := range rpc.sent[1:] {
		require.Equal(t, original.Nonce(), tx.Nonce())
		require.Equal(t, firstReplacement.GasFeeCap(), tx.GasFeeCap())
		require.Equal(t, firstReplacement.GasTipCap(), tx.GasTipCap())
	}
}

func TestLostResponseUsesObservedPendingNonce(t *testing.T) {
	c, rpc := newSubmitter(t)
	rpc.send = func(tx *types.Transaction) error {
		rpc.pending = tx.Nonce() + 1
		return context.DeadlineExceeded
	}
	require.ErrorIs(t, submit(t, c), context.DeadlineExceeded)
	require.Zero(t, c.nextNonce)
	rpc.send = nil
	require.NoError(t, submit(t, c))
	require.Equal(t, uint64(1), rpc.sent[1].Nonce())
}

func TestAlreadyKnownTransactionAdvancesNonce(t *testing.T) {
	c, rpc := newSubmitter(t)
	rpc.send = func(*types.Transaction) error { return errors.New("already known") }
	require.NoError(t, submit(t, c))
	require.Equal(t, uint64(1), c.nextNonce)
	require.NoError(t, submit(t, c))
	require.Equal(t, uint64(1), rpc.sent[1].Nonce())
}

func TestRestartRecoversUnknownGap(t *testing.T) {
	c, rpc := newSubmitter(t)
	rpc.mined, rpc.pending = 7, 9
	require.NoError(t, submit(t, c))
	require.Equal(t, uint64(9), rpc.sent[0].Nonce())
	restarted, err := New(chainIDEth, rpc, c.signer, c.opts)
	require.NoError(t, err)
	_, _, err = restarted.selectNonce(t.Context(), time.Now())
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
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	submission, err := c.Submit(ctx, testIntent())
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Nil(t, submission)
	require.Empty(t, rpc.sent)
	c.signer = key
	require.NoError(t, submit(t, c))
	require.Zero(t, rpc.sent[0].Nonce())
}

func TestSubmissionUsesNodeFeeEstimate(t *testing.T) {
	for _, tt := range []struct {
		name             string
		baseFee, tip     int64
		wantFee, wantTip int64
	}{
		{name: "standard fees", baseFee: 100, tip: 10, wantFee: 210, wantTip: 10},
		{name: "zero fees"},
		{
			name: "above old fixed cap", baseFee: 20_000_000_000, tip: 1_000_000_000,
			wantFee: 41_000_000_000, wantTip: 1_000_000_000,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, rpc := newSubmitter(t)
			rpc.baseFee, rpc.tip = tt.baseFee, tt.tip
			require.NoError(t, submit(t, c))
			require.Len(t, rpc.sent, 1)
			require.Equal(t, big.NewInt(tt.wantFee), rpc.sent[0].GasFeeCap())
			require.Equal(t, big.NewInt(tt.wantTip), rpc.sent[0].GasTipCap())
			require.Equal(t, 1, rpc.tipCalls)
		})
	}
}

func TestStalledReplacementRefreshesFeeCeiling(t *testing.T) {
	c, rpc := newSubmitter(t)
	rpc.send = func(*types.Transaction) error { return errors.New("replacement transaction underpriced") }
	require.ErrorContains(t, submit(t, c), "still underpriced")
	rpc.send = nil
	for i, prices := range []struct{ baseFee, tip, wantFee int64 }{
		{100, 10, 840},
		{200, 20, 1008},
		{50, 5, 420},
		{50, 5, 420},
		{0, 2, 8},
	} {
		rpc.baseFee, rpc.tip = prices.baseFee, prices.tip
		require.NoError(t, submit(t, c))
		require.Len(t, rpc.sent, maxBroadcastAttempts+i+1)
		tx := rpc.sent[maxBroadcastAttempts+i]
		require.Zero(t, tx.Nonce())
		require.Equal(t, big.NewInt(prices.wantFee), tx.GasFeeCap())
		require.LessOrEqual(t, tx.GasTipCap().Cmp(tx.GasFeeCap()), 0)
		expireNonce(c)
	}
	require.Equal(t, 6, rpc.tipCalls)
}

func TestUnderpricedRetriesStopAtFeeCeiling(t *testing.T) {
	c, rpc := newSubmitter(t)
	rpc.baseFee, rpc.tip = 0, 2
	underpriced := errors.New("replacement transaction underpriced")
	rpc.send = func(*types.Transaction) error {
		rpc.baseFee, rpc.tip = 1, 4
		return underpriced
	}

	submission, err := c.Submit(t.Context(), testIntent())
	require.ErrorIs(t, err, underpriced)
	require.Nil(t, submission)
	require.ErrorContains(t, err, "fee ceiling 8 wei (4x fresh estimate)")
	require.Len(t, rpc.sent, 6)
	require.Equal(t, 1, rpc.tipCalls)
	for i, tx := range rpc.sent {
		require.Zero(t, tx.Nonce())
		require.LessOrEqual(t, tx.GasFeeCap().Int64(), int64(8))
		if i > 0 {
			require.Positive(t, tx.GasTipCap().Cmp(rpc.sent[i-1].GasTipCap()))
		}
	}
	last := rpc.sent[5]
	require.Equal(t, big.NewInt(8), last.GasFeeCap())
	require.Equal(t, big.NewInt(8), last.GasTipCap())

	// The next submission gets a fresh ceiling and can increase fees again.
	rpc.send = nil
	require.NoError(t, submit(t, c))
	require.Len(t, rpc.sent, 7)
	require.Equal(t, big.NewInt(10), rpc.sent[6].GasFeeCap())
	require.Equal(t, big.NewInt(10), rpc.sent[6].GasTipCap())
	require.Equal(t, 2, rpc.tipCalls)
}

func TestNonceQueryFailureDoesNotBroadcast(t *testing.T) {
	c, rpc := newSubmitter(t)
	rpc.nonceErr = errors.New("unavailable")
	require.ErrorIs(t, submit(t, c), rpc.nonceErr)
	require.Empty(t, rpc.sent)
}

func TestLaggingHeadDoesNotResetNonceRecovery(t *testing.T) {
	c, rpc := newSubmitter(t)
	rpc.mined, rpc.pending = 7, 7
	for range maxInFlight {
		require.NoError(t, submit(t, c))
	}
	require.ErrorIs(t, submit(t, c), ErrBusy)
	expireNonce(c)
	lowestUnminedNoncePendingSince := c.lowestUnminedNoncePendingSince
	feeCap := new(big.Int).Set(c.lastAttemptedFeeCap)
	tipCap := new(big.Int).Set(c.lastAttemptedTipCap)

	for range 3 {
		rpc.height, rpc.mined = 99, 6
		require.ErrorContains(t, submit(t, c), "behind last nonce observation")
		require.Equal(t, uint64(7), c.lowestUnminedNonce)
		require.Equal(t, lowestUnminedNoncePendingSince, c.lowestUnminedNoncePendingSince)
		require.Equal(t, feeCap, c.lastAttemptedFeeCap)
		require.Equal(t, tipCap, c.lastAttemptedTipCap)

		rpc.height, rpc.mined = 100, 7
		nonce, _, err := c.selectNonce(t.Context(), time.Now())
		require.NoError(t, err)
		require.Equal(t, uint64(7), nonce)
	}

	require.Len(t, rpc.sent, maxInFlight)
	require.NoError(t, submit(t, c))
	require.Equal(t, uint64(7), rpc.sent[maxInFlight].Nonce())
	require.Positive(t, rpc.sent[maxInFlight].GasFeeCap().Cmp(feeCap))
}

func TestFailedNonceQueryDoesNotAdvanceObservedHeight(t *testing.T) {
	c, rpc := newSubmitter(t)
	require.NoError(t, submit(t, c))
	rpc.height = 101
	rpc.nonceErr = errors.New("unavailable")
	require.ErrorIs(t, submit(t, c), rpc.nonceErr)
	rpc.height = 100
	rpc.nonceErr = nil
	require.NoError(t, submit(t, c))
	require.Equal(t, uint64(1), rpc.sent[1].Nonce())
}

func TestReorgAllowsLowestNonceReuse(t *testing.T) {
	for _, tt := range []struct {
		name   string
		height uint64
	}{
		{name: "same height", height: 101},
		{name: "higher height", height: 102},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, rpc := newSubmitter(t)
			rpc.mined, rpc.pending = 7, 7
			require.NoError(t, submit(t, c))
			rpc.height, rpc.mined, rpc.pending = 101, 8, 8
			require.NoError(t, submit(t, c))
			expireNonce(c)
			rpc.height, rpc.mined, rpc.pending = tt.height, 7, 7
			now := time.Now()
			nonce, _, err := c.selectNonce(t.Context(), now)
			require.NoError(t, err)
			require.Equal(t, uint64(9), nonce)
			require.Equal(t, now, c.lowestUnminedNoncePendingSince)
			expireNonce(c)
			require.NoError(t, submit(t, c))
			require.Equal(t, uint64(7), rpc.sent[2].Nonce())
		})
	}
}

func TestDroppedNonceRecoversBelowLocalAllocation(t *testing.T) {
	c, rpc := newSubmitter(t)
	rpc.mined, rpc.pending = 7, 7
	for range 3 {
		require.NoError(t, submit(t, c))
	}
	require.Equal(t, uint64(10), c.nextNonce)
	rpc.pending = 7
	nonce, _, err := c.selectNonce(t.Context(), time.Now())
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
				sentAt = sentAt.Add(-2 * c.opts.PendingTxTimeout)
			}
			retry, err := c.ShouldRetry(t.Context(), common.Hash{}.Hex(), sentAt)
			require.ErrorIs(t, err, tt.wantErr)
			require.Equal(t, tt.retry, retry)
		})
	}
}

type revertError string

func (revertError) Error() string    { return "execution reverted" }
func (revertError) ErrorCode() int   { return -32000 }
func (e revertError) ErrorData() any { return string(e) }

func TestSubmitRejectsEstimationRevert(t *testing.T) {
	submitter, eth := newSubmitter(t)
	contractABI, err := besuqbft.ContractMetaData.GetAbi()
	require.NoError(t, err)
	customErr := contractABI.Errors["ConsensusStateExpired"]
	args, err := customErr.Inputs.Pack(uint64(100), big.NewInt(121), uint64(20))
	require.NoError(t, err)
	revert := revertError(hexutil.Encode(append(customErr.ID[:4], args...)))
	eth.gasErr = revert
	result, err := submitter.Submit(t.Context(), testIntent())
	require.Nil(t, result)
	require.ErrorIs(t, err, revert)
	require.EqualError(t, err, "estimating gas: ConsensusStateExpired(100, 121, 20): execution reverted")
	require.Empty(t, eth.sent)
	require.Zero(t, submitter.nextNonce)
}
