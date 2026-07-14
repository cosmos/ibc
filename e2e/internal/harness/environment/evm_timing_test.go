package environment

import (
	"context"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"

	chainevm "github.com/cosmos/ibc/e2e/internal/harness/chain/evm"
)

func TestResolvedEVMWaitNextPendingTxUsesCompletionBudget(t *testing.T) {
	service := &timedEthService{}
	evmAccess := resolvedEVMForTimingTest(t, Timing{
		CompletionBudget: 30 * time.Millisecond,
		PollInterval:     5 * time.Millisecond,
	}, service)

	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()
	started := time.Now()
	err := evmAccess.WaitNextPendingTx(ctx)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(started), 250*time.Millisecond)
	require.GreaterOrEqual(t, service.pendingCallCount(), 2)
}

func TestResolvedEVMWaitNextPendingTxUsesPollInterval(t *testing.T) {
	const readyAfter = 5
	service := &timedEthService{pendingReadyAfter: readyAfter}
	evmAccess := resolvedEVMForTimingTest(t, Timing{
		CompletionBudget: 250 * time.Millisecond,
		PollInterval:     20 * time.Millisecond,
	}, service)

	started := time.Now()
	err := evmAccess.WaitNextPendingTx(t.Context())

	require.NoError(t, err)
	require.Equal(t, readyAfter, service.pendingCallCount())
	require.GreaterOrEqual(t, time.Since(started), 40*time.Millisecond)
}

func TestResolvedEVMBroadcastUsesCompletionBudget(t *testing.T) {
	service := &timedEthService{}
	evmAccess := resolvedEVMForTimingTest(t, Timing{
		CompletionBudget: 30 * time.Millisecond,
		PollInterval:     5 * time.Millisecond,
	}, service)
	sender, err := chainevm.AccountFromHex(
		"0000000000000000000000000000000000000000000000000000000000000007",
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err = evmAccess.BroadcastTx(ctx, sender, nil, []byte{0x00}, nil)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(started), 250*time.Millisecond)
}

func TestResolvedEVMBroadcastUsesPollIntervalWithinCompletionBudget(t *testing.T) {
	const readyAfter = 8
	service := &timedEthService{receiptReadyAfter: readyAfter}
	evmAccess := resolvedEVMForTimingTest(t, Timing{
		CompletionBudget: 250 * time.Millisecond,
		PollInterval:     20 * time.Millisecond,
	}, service)
	sender, err := chainevm.AccountFromHex(
		"0000000000000000000000000000000000000000000000000000000000000007",
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(t.Context(), 400*time.Millisecond)
	defer cancel()
	started := time.Now()
	receipt, err := evmAccess.BroadcastTx(ctx, sender, nil, []byte{0x00}, nil)

	require.NoError(t, err)
	require.Equal(t, types.ReceiptStatusSuccessful, receipt.Status)
	require.Equal(t, readyAfter, service.receiptCallCount())
	require.GreaterOrEqual(t, time.Since(started), 100*time.Millisecond)
}

type timingEVMAdapter struct {
	chainevm.Identity
	client *chainevm.EVMClient
}

func (a *timingEVMAdapter) EVM() *chainevm.EVMClient { return a.client }

func (a *timingEVMAdapter) Height(ctx context.Context) (uint64, error) {
	return a.client.Height(ctx)
}

func resolvedEVMForTimingTest(t *testing.T, timing Timing, service *timedEthService) *EVM {
	t.Helper()

	server := rpc.NewServer()
	require.NoError(t, server.RegisterName("eth", service))
	rpcClient := rpc.DialInProc(server)
	client, err := chainevm.NewConnectedClient(t.Context(), ethclient.NewClient(rpcClient))
	require.NoError(t, err)
	t.Cleanup(func() {
		client.Close()
		server.Stop()
	})

	chain := &Chain{
		id:     "timed-chain",
		timing: timing,
		impl:   &timingEVMAdapter{Identity: chainevm.NewIdentity("timed-chain", "in-process"), client: client},
	}
	evmAccess, err := chain.EVM()
	require.NoError(t, err)
	return evmAccess
}

type timedEthService struct {
	mu sync.Mutex

	pendingCalls      int
	pendingReadyAfter int
	receiptCalls      int
	receiptReadyAfter int
}

func (*timedEthService) ChainId() hexutil.Uint64 { //nolint:revive // JSON-RPC requires eth_chainId.
	return 31337
}

func (s *timedEthService) GetBlockTransactionCountByNumber(string) hexutil.Uint {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingCalls++
	if s.pendingReadyAfter > 0 && s.pendingCalls >= s.pendingReadyAfter {
		return 1
	}
	return 0
}

func (*timedEthService) GetTransactionCount(common.Address, string) hexutil.Uint64 { return 0 }

func (*timedEthService) EstimateGas(map[string]any) hexutil.Uint64 { return 21_000 }

func (*timedEthService) MaxPriorityFeePerGas() *hexutil.Big {
	return (*hexutil.Big)(big.NewInt(1_000_000_000))
}

func (*timedEthService) GetBlockByNumber(string, bool) *types.Header {
	return &types.Header{
		UncleHash:   types.EmptyUncleHash,
		TxHash:      types.EmptyTxsHash,
		ReceiptHash: types.EmptyReceiptsHash,
		Difficulty:  big.NewInt(1),
		Number:      big.NewInt(1),
		GasLimit:    30_000_000,
		Time:        1,
		BaseFee:     big.NewInt(1),
	}
}

func (*timedEthService) SendRawTransaction(raw hexutil.Bytes) (common.Hash, error) {
	var tx types.Transaction
	if err := tx.UnmarshalBinary(raw); err != nil {
		return common.Hash{}, err
	}
	return tx.Hash(), nil
}

func (s *timedEthService) GetTransactionReceipt(hash common.Hash) *types.Receipt {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.receiptCalls++
	if s.receiptReadyAfter == 0 || s.receiptCalls < s.receiptReadyAfter {
		return nil
	}
	return &types.Receipt{
		Type:              types.DynamicFeeTxType,
		Status:            types.ReceiptStatusSuccessful,
		CumulativeGasUsed: 21_000,
		Logs:              []*types.Log{},
		TxHash:            hash,
		GasUsed:           21_000,
		EffectiveGasPrice: big.NewInt(1_000_000_001),
		BlockHash:         common.HexToHash("0x1"),
		BlockNumber:       big.NewInt(1),
	}
}

func (s *timedEthService) pendingCallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pendingCalls
}

func (s *timedEthService) receiptCallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.receiptCalls
}
