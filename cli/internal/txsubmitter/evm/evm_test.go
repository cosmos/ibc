// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"math/big"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besuqbft"
	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/internal/service/signer"
	"github.com/cosmos/ibc/cli/internal/store"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

const (
	chainIDEth = "1"
	toAddress  = "0xe20BccD900Fa1B48f46F5a483d9De063b07eDFCC"
)

type testRPC struct {
	client             *rpc.Client
	nonceBlock         rpc.BlockNumberOrHash
	afterNonce         func()
	reportedHash       common.Hash
	mu                 sync.Mutex
	head               *types.Header
	mined              uint64
	sent               []*types.Transaction
	receipts           map[common.Hash]*types.Receipt
	sendErr, errorRead error
	estimateErr        error
	receiptErr         error
	beforeSend         func(*types.Transaction)
}

func newRPC(t *testing.T) *testRPC {
	t.Helper()
	r := &testRPC{
		receipts: make(map[common.Hash]*types.Receipt),
		head:     &types.Header{Number: big.NewInt(10), BaseFee: big.NewInt(100), Time: 1000},
	}
	server := rpc.NewServer()
	t.Cleanup(server.Stop)
	require.NoError(t, server.RegisterName("eth", r))
	r.client = rpc.DialInProc(server)
	t.Cleanup(r.client.Close)
	return r
}

func (r *testRPC) Client() *rpc.Client { return r.client }

func (r *testRPC) GetBlockByNumber(_ context.Context, number hexutil.Uint64, _ bool) (map[string]common.Hash, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if uint64(number) > r.head.Number.Uint64() {
		return nil, nil
	}
	if r.reportedHash != (common.Hash{}) && uint64(number) == r.head.Number.Uint64() {
		return map[string]common.Hash{"hash": r.reportedHash}, r.errorRead
	}
	h := types.CopyHeader(r.head)
	h.Number = new(big.Int).SetUint64(uint64(number))
	return map[string]common.Hash{"hash": h.Hash()}, r.errorRead
}

func (r *testRPC) BalanceAt(context.Context, common.Address, *big.Int) (*big.Int, error) {
	return big.NewInt(1e18), nil
}

func (r *testRPC) HeaderByNumber(context.Context, *big.Int) (*types.Header, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return types.CopyHeader(r.head), r.errorRead
}

func (r *testRPC) SuggestGasTipCap(context.Context) (*big.Int, error) { return big.NewInt(10), nil }
func (r *testRPC) PendingCodeAt(_ context.Context, address common.Address) ([]byte, error) {
	if address == common.HexToAddress(toAddress) {
		return []byte{0x60}, nil
	}
	return nil, nil
}

func (r *testRPC) EstimateGas(context.Context, ethereum.CallMsg) (uint64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return 21000, r.estimateErr
}

func (r *testRPC) NonceAt(context.Context, common.Address, *big.Int) (uint64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.mined, r.errorRead
}

func (r *testRPC) GetTransactionCount(
	_ context.Context,
	_ common.Address,
	block rpc.BlockNumberOrHash,
) (hexutil.Uint64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nonceBlock = block
	canonical := r.head.Hash()
	if r.reportedHash != (common.Hash{}) {
		canonical = r.reportedHash
	}
	if block.BlockHash == nil || *block.BlockHash != canonical {
		return 0, ethereum.NotFound
	}
	nonce := r.mined
	if r.afterNonce != nil {
		r.afterNonce()
	}
	return hexutil.Uint64(nonce), r.errorRead
}

func (r *testRPC) SendTransaction(_ context.Context, tx *types.Transaction) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.beforeSend != nil {
		r.beforeSend(tx)
	}
	r.sent = append(r.sent, tx)
	return r.sendErr
}

func (r *testRPC) sendCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.sent)
}

func (r *testRPC) TransactionReceipt(_ context.Context, hash common.Hash) (*types.Receipt, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.receiptErr != nil {
		return nil, r.receiptErr
	}
	if r.errorRead != nil {
		return nil, r.errorRead
	}
	if receipt := r.receipts[hash]; receipt != nil {
		return receipt, nil
	}
	return nil, ethereum.NotFound
}

func newManager(t *testing.T) (*TxSubmitter, *testRPC, store.Store) {
	t.Helper()
	db, err := store.NewSqlite(filepath.Join(t.TempDir(), "journal.db"))
	require.NoError(t, err)
	_, err = db.MigrateUp()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	key, err := signer.GenerateLocalSecp256k1Signer()
	require.NoError(t, err)
	rpc := newRPC(t)
	c, err := New(chainIDEth, rpc, key, db, ChainOptions{PollInterval: time.Hour})
	require.NoError(t, err)
	c.ctx = t.Context()
	c.reconciler.reconcile(t.Context())
	return c, rpc, db
}

func pendingAttempts(t *testing.T, c *TxSubmitter) []store.TxAttempt {
	t.Helper()
	attempts, err := c.db.ListUnresolvedTxAttempts(t.Context(), c.chainID, c.address.Hex(), 0)
	require.NoError(t, err)
	return attempts
}

func packetIntent(t *testing.T, db store.Store, sequence uint64) v2.TxIntent {
	t.Helper()
	err := db.UpsertPacket(
		t.Context(),
		store.UpsertPacket{
			Status:                    store.RelayStatusPending,
			SourceChainID:             "2",
			DestinationChainID:        chainIDEth,
			SourceTxHash:              "0xsend",
			SourceTxTime:              time.Now(),
			PacketSequenceNumber:      sequence,
			PacketSourceClientID:      "source",
			PacketDestinationClientID: "dest",
			PacketTimeoutTimestamp:    time.Now().Add(time.Hour),
		},
	)
	require.NoError(t, err)
	return v2.TxIntent{
		To:   toAddress,
		Data: []byte{0xde, 0xad},
		Operations: []v2.OperationMetadata{
			{SourceChainID: "2", SourceClientID: "source", Sequence: sequence, Type: v2.OperationTypeRecv},
		},
	}
}

func TestPersistBeforeBroadcast(t *testing.T) {
	c, rpc, db := newManager(t)
	intent := packetIntent(t, db, 1)
	rpc.beforeSend = func(tx *types.Transaction) {
		a, err := db.GetTxAttempt(t.Context(), chainIDEth, tx.Hash().Hex())
		require.NoError(t, err)
		raw, err := tx.MarshalBinary()
		require.NoError(t, err)
		require.Equal(t, raw, a.SignedTx)
		operations, err := db.ListTxOperations(t.Context(), a.ID)
		require.NoError(t, err)
		require.Len(t, operations, 1)
		saved, err := db.ListPacketsBySourceTx(t.Context(), "2", "0xsend")
		require.NoError(t, err)
		require.Nil(t, saved[0].RecvTxHash)
	}
	rpc.sendErr = context.DeadlineExceeded
	err := c.submit(t.Context(), intent)
	require.NoError(t, err)
	require.Equal(t, uint64(0), rpc.sent[0].Nonce())
	require.Equal(t, big.NewInt(210), rpc.sent[0].GasFeeCap())
	require.Len(t, rpc.sent, 1)
	status, err := c.HasPending(t.Context(), intent.Operations[0])
	require.NoError(t, err)
	require.True(t, status)
}

func TestMixedOperationsSurviveRestartAndRepricing(t *testing.T) {
	c, rpc, db := newManager(t)
	intent := packetIntent(t, db, 1)
	ack := packetIntent(t, db, 2).Operations[0]
	ack.Type = v2.OperationTypeAck
	samePacketAck := intent.Operations[0]
	samePacketAck.Type = v2.OperationTypeAck
	intent.Operations = append(intent.Operations, ack, samePacketAck)
	require.NoError(t, c.Start())
	t.Cleanup(func() { require.NoError(t, c.Stop()) })
	require.NoError(t, c.Submit(t.Context(), intent))
	require.NoError(t, c.Stop())

	restarted, err := New(chainIDEth, rpc, c.signer, db, c.opts)
	require.NoError(t, err)
	for _, op := range intent.Operations {
		pending, lookupErr := restarted.HasPending(t.Context(), op)
		require.NoError(t, lookupErr)
		require.True(t, pending)
	}
	absent := ack
	absent.Type = v2.OperationTypeRecv
	pending, err := restarted.HasPending(t.Context(), absent)
	require.NoError(t, err)
	require.False(t, pending)

	restarted.opts.RepriceInterval = time.Nanosecond
	restarted.reconciler.reconcile(t.Context())
	attempts := pendingAttempts(t, restarted)
	require.Len(t, attempts, 2)
	want := []store.TxOperation{
		{
			PacketKey: store.PacketKey{SourceChainID: "2", SourceClientID: "source", Sequence: 1},
			Type:      store.OperationTypeRecv,
		},
		{
			PacketKey: store.PacketKey{SourceChainID: "2", SourceClientID: "source", Sequence: 2},
			Type:      store.OperationTypeAck,
		},
		{
			PacketKey: store.PacketKey{SourceChainID: "2", SourceClientID: "source", Sequence: 1},
			Type:      store.OperationTypeAck,
		},
	}
	for _, attempt := range attempts {
		operations, lookupErr := db.ListTxOperations(t.Context(), attempt.ID)
		require.NoError(t, lookupErr)
		require.ElementsMatch(t, want, operations)
	}
	require.Equal(t, rpc.sent[0].Data(), rpc.sent[1].Data())
	require.Equal(t, rpc.sent[0].Nonce(), rpc.sent[1].Nonce())
}

func saveIntentAtNonce(t *testing.T, c *TxSubmitter, intent v2.TxIntent, nonce uint64) *types.Transaction {
	t.Helper()
	tx, err := c.buildAndSignTransaction(t.Context(), intent.To, intent.Data, nonce)
	require.NoError(t, err)
	p := &pendingTx{}
	for _, op := range intent.Operations {
		p.operations = append(p.operations, store.TxOperation{
			PacketKey: store.PacketKey{
				SourceChainID: op.SourceChainID, SourceClientID: op.SourceClientID, Sequence: op.Sequence,
			},
			Type: op.Type.String(),
		})
	}
	require.NoError(t, saveAttempt(t.Context(), c.db, tx, store.TxAttempt{
		ChainID: c.chainID, Sender: c.address.Hex(), SubmittedAt: time.Now().UTC(),
	}, p))
	return p.latest().tx
}

func TestDatabaseFailureDoesNotBroadcast(t *testing.T) {
	c, rpc, db := newManager(t)
	intent := packetIntent(t, db, 1)
	intent.Operations[0].Sequence = 999
	err := c.submit(t.Context(), intent)
	require.ErrorContains(t, err, "not found")
	require.Empty(t, rpc.sent)
	attempts, err := db.ListUnresolvedTxAttempts(t.Context(), chainIDEth, c.address.Hex(), 0)
	require.NoError(t, err)
	require.Empty(t, attempts)
}

func TestReceiptFailureIsRetriedWithoutBlockingSubmission(t *testing.T) {
	c, rpc, db := newManager(t)
	first := packetIntent(t, db, 1)
	require.NoError(t, c.submit(t.Context(), first))
	tx := rpc.sent[0]
	rpc.receiptErr = errors.New("receipt service unavailable")
	rpc.mined = 1
	c.reconciler.reconcile(t.Context())
	pending, err := c.HasPending(t.Context(), first.Operations[0])
	require.NoError(t, err)
	require.True(t, pending)
	require.Len(t, pendingAttempts(t, c), 1)
	require.NoError(t, c.submit(t.Context(), packetIntent(t, db, 2)))
	require.Equal(t, uint64(1), rpc.sent[1].Nonce())

	rpc.receiptErr = nil
	rpc.receipts[tx.Hash()] = &types.Receipt{
		TxHash: tx.Hash(), BlockHash: rpc.head.Hash(), BlockNumber: rpc.head.Number,
		Status: types.ReceiptStatusSuccessful,
	}
	c.reconciler.reconcile(t.Context())
	attempt, err := db.GetTxAttempt(t.Context(), chainIDEth, tx.Hash().Hex())
	require.NoError(t, err)
	require.Equal(t, store.TxConfirmed, attempt.Status)
	require.Len(t, pendingAttempts(t, c), 1, "the second nonce still needs mining")
}

func TestSubmissionReadsNonceWithoutPolling(t *testing.T) {
	c, rpc, db := newManager(t)
	intent := packetIntent(t, db, 1)
	rpc.errorRead = errors.New("RPC unavailable")
	require.ErrorContains(t, c.submit(t.Context(), intent), "RPC unavailable")
	require.Empty(t, rpc.sent)
	rpc.errorRead = nil
	rpc.mined = 5
	require.NoError(t, c.submit(t.Context(), intent))
	require.Equal(t, uint64(5), rpc.sent[0].Nonce())
	require.NoError(t, c.submit(t.Context(), packetIntent(t, db, 2)))
	require.Equal(t, uint64(6), rpc.sent[1].Nonce(), "the journal must retain the previous allocation")
}

func TestNonceWindowAdvancesWhenMined(t *testing.T) {
	c, rpc, db := newManager(t)
	c.opts.MaxInFlight = 2
	require.NoError(t, c.submit(t.Context(), packetIntent(t, db, 1)))
	require.NoError(t, c.submit(t.Context(), packetIntent(t, db, 2)))
	third := packetIntent(t, db, 3)
	require.ErrorIs(t, c.submit(t.Context(), third), ErrBusy)
	rpc.mined = 1
	require.NoError(t, c.submit(t.Context(), third))
	require.Equal(t, uint64(2), rpc.sent[2].Nonce())
	require.Len(t, pendingAttempts(t, c), 3, "allocation does not wait for polling")
	c.reconciler.reconcile(t.Context())
	require.Len(t, pendingAttempts(t, c), 2)
}

func TestRepriceRetainsIntentWhenOlderAttemptWins(t *testing.T) {
	installTestMetrics(t)
	c, rpc, db := newManager(t)
	intent := packetIntent(t, db, 1)
	err := c.submit(t.Context(), intent)
	require.NoError(t, err)
	// Repricing must not discard already broadcast work on a later revert.
	rpc.estimateErr = errors.New("execution reverted")
	c.opts.RepriceInterval = time.Nanosecond
	c.reconciler.reconcile(t.Context())
	require.Len(t, rpc.sent, 2)
	old, newTx := rpc.sent[0], rpc.sent[1]
	require.Equal(t, old.Nonce(), newTx.Nonce())
	require.Equal(t, old.To(), newTx.To())
	require.Equal(t, old.Data(), newTx.Data())
	require.Equal(t, old.Value(), newTx.Value())
	require.Positive(t, newTx.GasTipCap().Cmp(old.GasTipCap()))
	rpc.mined = 1
	rpc.receipts[old.Hash()] = &types.Receipt{
		TxHash:            old.Hash(),
		BlockHash:         rpc.head.Hash(),
		BlockNumber:       rpc.head.Number,
		Status:            types.ReceiptStatusSuccessful,
		EffectiveGasPrice: big.NewInt(100),
		GasUsed:           21000,
	}
	c.reconciler.reconcile(t.Context())
	require.Empty(t, pendingAttempts(t, c))
	saved, err := db.ListPacketsBySourceTx(t.Context(), "2", "0xsend")
	require.NoError(t, err)
	require.Nil(t, saved[0].RecvTxHash)
	require.Equal(t, big.NewInt(2_100_000), gasTotalWei(t, metrics, chainIDEth, c.address.Hex()))

	status, err := c.HasPending(t.Context(), intent.Operations[0])
	require.NoError(t, err)
	require.False(t, status)
	winner, err := db.GetTxAttempt(t.Context(), chainIDEth, old.Hash().Hex())
	require.NoError(t, err)
	require.Equal(t, store.TxConfirmed, winner.Status)
	loser, err := db.GetTxAttempt(t.Context(), chainIDEth, newTx.Hash().Hex())
	require.NoError(t, err)
	require.Equal(t, store.TxReplaced, loser.Status)
	restarted, err := New(chainIDEth, rpc, c.signer, db, c.opts)
	require.NoError(t, err)
	status, err = restarted.HasPending(t.Context(), intent.Operations[0])
	require.NoError(t, err)
	require.False(t, status)
}

func TestAllocationStartsAtMinedNonce(t *testing.T) {
	c, rpc, db := newManager(t)
	rpc.mined = 2
	c.reconciler.reconcile(t.Context())
	require.Empty(t, rpc.sent)
	require.NoError(t, c.submit(t.Context(), packetIntent(t, db, 1)))
	require.Len(t, rpc.sent, 1)
	require.Equal(t, uint64(2), rpc.sent[0].Nonce())
}

func TestRestartPreservesRebroadcastTiming(t *testing.T) {
	c, rpc, db := newManager(t)
	rpc.sendErr = context.DeadlineExceeded
	require.NoError(t, c.submit(t.Context(), packetIntent(t, db, 1)))
	tx := rpc.sent[0]
	restarted, err := New(chainIDEth, rpc, c.signer, db, c.opts)
	require.NoError(t, err)
	restarted.ctx = t.Context()
	restarted.reconciler.reconcile(t.Context())
	require.Len(t, rpc.sent, 1, "restart must respect the last broadcast, even when its response was lost")

	rpc.sendErr = nil
	require.NoError(
		t,
		db.SetTxBroadcast(t.Context(), chainIDEth, tx.Hash().Hex(), time.Now().Add(-2*c.opts.PollInterval)),
	)
	restarted.reconciler.reconcile(t.Context())
	require.Len(t, rpc.sent, 2)
	require.Equal(t, tx.Hash(), rpc.sent[1].Hash())
	restarted.reconciler.reconcile(t.Context())
	require.Len(t, rpc.sent, 2, "rebroadcast must update the persisted timestamp")
}

func TestRestartBroadcastsSavedUnsentAttemptBeforeRepricing(t *testing.T) {
	c, rpc, db := newManager(t)
	c.db = &uncertainCommitStore{Store: db, uncertain: true}
	c.reconciler.db = c.db
	require.ErrorContains(t, c.submit(t.Context(), packetIntent(t, db, 1)), "lost commit response")
	require.Empty(t, rpc.sent)
	a := pendingAttempts(t, c)[0]
	require.Nil(t, a.LastBroadcastAt)

	restarted, err := New(chainIDEth, rpc, c.signer, db, c.opts)
	require.NoError(t, err)
	restarted.ctx = t.Context()
	restarted.opts.RepriceInterval = time.Nanosecond
	restarted.reconciler.reconcile(t.Context())
	require.Len(t, rpc.sent, 1)
	require.Equal(t, a.Hash, rpc.sent[0].Hash().Hex())
	require.NotNil(t, pendingAttempts(t, restarted)[0].LastBroadcastAt)
}

type failedBroadcastStore struct{ store.Store }

func (s failedBroadcastStore) SetTxBroadcast(context.Context, string, string, time.Time) error {
	return errors.New("lost broadcast record")
}

func TestRestartRecoversBroadcastWithoutTimestamp(t *testing.T) {
	c, rpc, db := newManager(t)
	c.db = failedBroadcastStore{Store: db}
	c.reconciler.db = c.db
	require.NoError(t, c.submit(t.Context(), packetIntent(t, db, 1)))
	require.Nil(t, pendingAttempts(t, c)[0].LastBroadcastAt)
	restarted, err := New(chainIDEth, rpc, c.signer, db, c.opts)
	require.NoError(t, err)
	restarted.ctx = t.Context()
	restarted.reconciler.reconcile(t.Context())
	require.Len(t, rpc.sent, 2)
	require.Equal(t, rpc.sent[0].Hash(), rpc.sent[1].Hash())
	require.NotNil(t, pendingAttempts(t, restarted)[0].LastBroadcastAt)
}

func TestFeeCapRetainsBlockedWork(t *testing.T) {
	c, rpc, db := newManager(t)
	err := c.submit(t.Context(), packetIntent(t, db, 1))
	require.NoError(t, err)
	c.opts.MaxFeePerGas = big.NewInt(210)
	c.opts.RepriceInterval = time.Nanosecond
	c.reconciler.reconcile(t.Context())
	require.Len(t, pendingAttempts(t, c), 1)
	require.Len(t, rpc.sent, 1)
}

func TestConcurrentSubmissionsAllocateDistinctNonces(t *testing.T) {
	c, _, db := newManager(t)
	c.opts.PollInterval = time.Millisecond
	intents := make([]v2.TxIntent, 8)
	for i := range intents {
		intents[i] = packetIntent(t, db, uint64(i+1))
	}
	require.NoError(t, c.Start())
	t.Cleanup(func() { require.NoError(t, c.Stop()) })
	results := make(chan error, len(intents))
	for _, intent := range intents {
		go func() {
			err := c.Submit(t.Context(), intent)
			results <- err
		}()
	}
	for range intents {
		require.NoError(t, <-results)
	}
	attempts := pendingAttempts(t, c)
	require.Len(t, attempts, len(intents))
	seen := map[uint64]bool{}
	for _, attempt := range attempts {
		require.False(t, seen[attempt.Nonce])
		seen[attempt.Nonce] = true
	}
}

func TestExternalTransactionConsumesNonce(t *testing.T) {
	c, rpc, db := newManager(t)
	rpc.sendErr = errors.New("replacement transaction underpriced")
	intent := packetIntent(t, db, 1)
	require.NoError(t, c.submit(t.Context(), intent))
	require.Equal(t, store.TxPending, pendingAttempts(t, c)[0].Status)
	c.reconciler.reconcile(t.Context())
	require.Len(t, rpc.sent, 1)

	restarted, err := New(chainIDEth, rpc, c.signer, db, c.opts)
	require.NoError(t, err)
	restarted.ctx = t.Context()
	restarted.reconciler.reconcile(t.Context())
	require.Len(t, rpc.sent, 1)
	status, err := restarted.HasPending(t.Context(), intent.Operations[0])
	require.NoError(t, err)
	require.True(t, status)

	rpc.mined = 1
	restarted.reconciler.reconcile(t.Context())
	require.Len(t, rpc.sent, 1)
	status, err = restarted.HasPending(t.Context(), intent.Operations[0])
	require.NoError(t, err)
	require.False(t, status)
	a, err := db.GetTxAttempt(t.Context(), chainIDEth, rpc.sent[0].Hash().Hex())
	require.NoError(t, err)
	require.Equal(t, store.TxConsumed, a.Status)

	rpc.sendErr = nil
	require.NoError(t, restarted.submit(t.Context(), intent))
	require.Len(t, rpc.sent, 2)
	require.Equal(t, uint64(1), rpc.sent[1].Nonce())
}

func TestUnderpricedAttemptIsRepricedAfterRestart(t *testing.T) {
	c, rpc, db := newManager(t)
	rpc.sendErr = errors.New("replacement transaction underpriced")
	intent := packetIntent(t, db, 1)
	require.NoError(t, c.submit(t.Context(), intent))
	first := rpc.sent[0]
	rpc.sendErr = nil
	restarted, err := New(chainIDEth, rpc, c.signer, db, c.opts)
	require.NoError(t, err)
	restarted.ctx = t.Context()
	restarted.opts.RepriceInterval = time.Nanosecond
	restarted.reconciler.reconcile(t.Context())
	require.Len(t, rpc.sent, 2)
	replacement := rpc.sent[1]
	require.Equal(t, first.Nonce(), replacement.Nonce())
	require.Equal(t, first.To(), replacement.To())
	require.Equal(t, first.Data(), replacement.Data())
	require.Positive(t, replacement.GasFeeCap().Cmp(first.GasFeeCap()))
	require.Positive(t, replacement.GasTipCap().Cmp(first.GasTipCap()))
	rpc.mined = 1
	rpc.receipts[replacement.Hash()] = &types.Receipt{
		TxHash: replacement.Hash(), BlockHash: rpc.head.Hash(), BlockNumber: rpc.head.Number,
		Status: types.ReceiptStatusSuccessful,
	}
	restarted.reconciler.reconcile(t.Context())
	status, err := restarted.HasPending(t.Context(), intent.Operations[0])
	require.NoError(t, err)
	require.False(t, status)
}

func TestReceiptAloneDoesNotResolveNonce(t *testing.T) {
	c, rpc, db := newManager(t)
	intent := packetIntent(t, db, 1)
	err := c.submit(t.Context(), intent)
	require.NoError(t, err)
	tx := rpc.sent[0]
	rpc.receipts[tx.Hash()] = &types.Receipt{
		TxHash:      tx.Hash(),
		BlockHash:   common.HexToHash("0xdead"),
		BlockNumber: rpc.head.Number,
		Status:      types.ReceiptStatusSuccessful,
	}
	c.reconciler.reconcile(t.Context())
	require.Len(t, pendingAttempts(t, c), 1)
	status, err := c.HasPending(t.Context(), intent.Operations[0])
	require.NoError(t, err)
	require.True(t, status)
}

func TestInvalidReceiptKeepsAttemptPending(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(*types.Receipt)
	}{
		{"missing block", func(r *types.Receipt) { r.BlockNumber = nil }},
		{"wrong transaction", func(r *types.Receipt) { r.TxHash = common.HexToHash("0xbad") }},
		{"orphaned", func(r *types.Receipt) { r.BlockHash = common.HexToHash("0xdead") }},
		{"invalid status", func(r *types.Receipt) { r.Status = 2 }},
		{"newer than snapshot", func(r *types.Receipt) { r.BlockNumber = new(big.Int).Add(r.BlockNumber, big.NewInt(1)) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, rpc, db := newManager(t)
			require.NoError(t, c.submit(t.Context(), packetIntent(t, db, 1)))
			tx := rpc.sent[0]
			rpc.mined = 1
			receipt := types.Receipt{
				TxHash: tx.Hash(), BlockHash: rpc.head.Hash(), BlockNumber: rpc.head.Number,
				Status: types.ReceiptStatusSuccessful,
			}
			invalid := receipt
			tt.mutate(&invalid)
			rpc.receipts[tx.Hash()] = &invalid
			c.reconciler.reconcile(t.Context())
			require.Len(t, pendingAttempts(t, c), 1)

			rpc.receipts[tx.Hash()] = &receipt
			c.reconciler.reconcile(t.Context())
			attempt, err := db.GetTxAttempt(t.Context(), chainIDEth, tx.Hash().Hex())
			require.NoError(t, err)
			require.Equal(t, store.TxConfirmed, attempt.Status)
		})
	}
}

type receiptErrorRPC struct {
	ETHClient
	hash common.Hash
}

func (r receiptErrorRPC) TransactionReceipt(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
	if hash == r.hash {
		return nil, errors.New("receipt service unavailable")
	}
	return r.ETHClient.TransactionReceipt(ctx, hash)
}

func TestReceiptErrorDoesNotHideWinningReplacement(t *testing.T) {
	c, rpc, db := newManager(t)
	require.NoError(t, c.submit(t.Context(), packetIntent(t, db, 1)))
	c.opts.RepriceInterval = time.Nanosecond
	c.reconciler.reconcile(t.Context())
	require.Len(t, rpc.sent, 2)
	old, winner := rpc.sent[0], rpc.sent[1]
	c.reconciler.eth = receiptErrorRPC{ETHClient: rpc, hash: old.Hash()}
	rpc.mined = 1
	rpc.receipts[winner.Hash()] = &types.Receipt{
		TxHash: winner.Hash(), BlockHash: rpc.head.Hash(), BlockNumber: rpc.head.Number,
		Status: types.ReceiptStatusSuccessful,
	}
	c.reconciler.reconcile(t.Context())
	for hash, status := range map[common.Hash]string{old.Hash(): store.TxReplaced, winner.Hash(): store.TxConfirmed} {
		attempt, err := db.GetTxAttempt(t.Context(), chainIDEth, hash.Hex())
		require.NoError(t, err)
		require.Equal(t, status, attempt.Status)
	}
}

func TestRepeatedAndOverlappingSubmissionsUseDistinctNonces(t *testing.T) {
	c, _, db := newManager(t)
	first := packetIntent(t, db, 1)
	second := packetIntent(t, db, 2)
	second.Operations = append(second.Operations, first.Operations...)
	first.Operations = append(first.Operations, packetIntent(t, db, 3).Operations...)
	require.NoError(t, c.Start())
	t.Cleanup(func() { require.NoError(t, c.Stop()) })
	require.NoError(t, c.Submit(t.Context(), first))
	require.NoError(t, c.Submit(t.Context(), second))
	require.NoError(t, c.Submit(t.Context(), first))

	attempts := pendingAttempts(t, c)
	require.Len(t, attempts, 3)
	for i, attempt := range attempts {
		require.Equal(t, uint64(i), attempt.Nonce)
	}
	pending, err := c.HasPending(t.Context(), first.Operations[0])
	require.NoError(t, err)
	require.True(t, pending)
}

type stalledSigner struct {
	signer.Signer
	started chan struct{}
	release chan struct{}
}

func (s stalledSigner) Sign(ctx context.Context, digest []byte) ([]byte, error) {
	if s.started != nil {
		select {
		case s.started <- struct{}{}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.release:
		return s.Signer.Sign(ctx, digest)
	}
}

func TestSignerDeadlineDoesNotConsumeNonce(t *testing.T) {
	c, rpc, db := newManager(t)
	key := c.signer
	c.signer = stalledSigner{Signer: key}
	c.reconciler.signer = c.signer
	c.opts.RequestTimeout = 100 * time.Millisecond
	intent := packetIntent(t, db, 1)
	err := c.submit(t.Context(), intent)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Empty(t, rpc.sent)
	c.signer = key
	c.reconciler.signer = c.signer
	c.opts.RequestTimeout = time.Second
	err = c.submit(t.Context(), intent)
	require.NoError(t, err)
	require.Equal(t, uint64(0), rpc.sent[0].Nonce())
}

func TestCallerCancellationBeforeJournalDoesNotBroadcast(t *testing.T) {
	c, rpc, db := newManager(t)
	key := stalledSigner{Signer: c.signer, started: make(chan struct{}, 2), release: make(chan struct{})}
	c.signer = key
	c.reconciler.signer = c.signer
	intent := packetIntent(t, db, 1)
	require.NoError(t, c.Start())
	t.Cleanup(func() { require.NoError(t, c.Stop()) })
	caller, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		err := c.Submit(caller, intent)
		result <- err
	}()
	select {
	case <-key.started:
	case <-time.After(time.Second):
		t.Fatal("signing not started")
	}
	status, err := c.HasPending(t.Context(), intent.Operations[0])
	require.NoError(t, err)
	require.False(t, status, "unsigned work is not journaled")
	cancel()
	require.ErrorIs(t, <-result, context.Canceled)
	require.Empty(t, pendingAttempts(t, c))
	require.Zero(t, rpc.sendCount())
	status, err = c.HasPending(t.Context(), intent.Operations[0])
	require.NoError(t, err)
	require.False(t, status)
	close(key.release)
	require.NoError(t, c.Submit(t.Context(), intent))
	require.Equal(t, uint64(0), pendingAttempts(t, c)[0].Nonce)
}

func TestCancellationWhileWaitingDoesNotSubmit(t *testing.T) {
	c, _, db := newManager(t)
	key := stalledSigner{Signer: c.signer, started: make(chan struct{}, 2), release: make(chan struct{})}
	c.signer = key
	c.reconciler.signer = c.signer
	first := packetIntent(t, db, 1)
	second := packetIntent(t, db, 2)
	require.NoError(t, c.Start())
	t.Cleanup(func() { require.NoError(t, c.Stop()) })
	result := make(chan error, 1)
	go func() { result <- c.Submit(t.Context(), first) }()
	select {
	case <-key.started:
	case <-time.After(time.Second):
		t.Fatal("signing not started")
	}
	caller, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	secondResult := make(chan error, 1)
	go func() { secondResult <- c.Submit(caller, second) }()
	<-caller.Done()
	close(key.release)
	require.NoError(t, <-result)
	require.ErrorIs(t, <-secondResult, context.DeadlineExceeded)
	status, err := c.HasPending(t.Context(), second.Operations[0])
	require.NoError(t, err)
	require.False(t, status)
	require.Len(t, pendingAttempts(t, c), 1)
	require.NoError(t, c.Submit(t.Context(), second))
	require.Equal(t, uint64(1), pendingAttempts(t, c)[1].Nonce)
}

func TestStopCancelsSubmission(t *testing.T) {
	c, rpc, db := newManager(t)
	key := stalledSigner{Signer: c.signer, started: make(chan struct{}), release: make(chan struct{})}
	c.signer = key
	c.reconciler.signer = c.signer
	intent := packetIntent(t, db, 1)
	require.NoError(t, c.Start())
	result := make(chan error, 1)
	go func() { result <- c.Submit(t.Context(), intent) }()
	select {
	case <-key.started:
	case <-time.After(time.Second):
		t.Fatal("signing not started")
	}
	require.NoError(t, c.Stop())
	require.ErrorIs(t, <-result, context.Canceled)
	require.ErrorIs(t, c.Submit(t.Context(), intent), context.Canceled)
	require.Empty(t, pendingAttempts(t, c))
	require.Zero(t, rpc.sendCount())
}

type cancelAfterCommitStore struct {
	store.Store
	cancel context.CancelFunc
}

func (s cancelAfterCommitStore) Transact(ctx context.Context, f func(store.Repository) error) error {
	err := s.Store.Transact(ctx, f)
	if err == nil {
		s.cancel()
	}
	return err
}

func TestCancellationAfterJournalSurvivesRestart(t *testing.T) {
	c, rpc, db := newManager(t)
	caller, cancel := context.WithCancel(t.Context())
	defer cancel()
	c.db = cancelAfterCommitStore{Store: db, cancel: cancel}
	c.reconciler.db = c.db
	intent := packetIntent(t, db, 1)
	require.NoError(t, c.submit(caller, intent))
	require.ErrorIs(t, caller.Err(), context.Canceled)
	require.Zero(t, rpc.sendCount())
	attempts := pendingAttempts(t, c)
	require.Len(t, attempts, 1)
	require.Nil(t, attempts[0].LastBroadcastAt)
	restarted, err := New(chainIDEth, rpc, c.signer, db, c.opts)
	require.NoError(t, err)
	require.NoError(t, restarted.Start())
	t.Cleanup(func() { require.NoError(t, restarted.Stop()) })
	require.Eventually(t, func() bool { return rpc.sendCount() == 1 }, time.Second, time.Millisecond)
	rpc.mu.Lock()
	hash := rpc.sent[0].Hash().Hex()
	rpc.mu.Unlock()
	require.Equal(t, attempts[0].Hash, hash)
	status, err := restarted.HasPending(t.Context(), intent.Operations[0])
	require.NoError(t, err)
	require.True(t, status)
}

type failedJournalReadStore struct {
	store.Store
	once sync.Once
}

func (s *failedJournalReadStore) ListUnresolvedTxAttempts(
	ctx context.Context,
	chainID, sender string,
	minedNonce uint64,
) ([]store.TxAttempt, error) {
	var err error
	s.once.Do(func() { err = errors.New("journal unavailable") })
	if err != nil {
		return nil, err
	}
	return s.Store.ListUnresolvedTxAttempts(ctx, chainID, sender, minedNonce)
}

func TestStartupRetriesJournalReadFailure(t *testing.T) {
	c, rpc, db := newManager(t)
	intent := packetIntent(t, db, 1)
	require.NoError(t, c.submit(t.Context(), intent))
	rpc.mined = 1
	c.db = &failedJournalReadStore{Store: db}
	c.reconciler.db = c.db
	c.opts.PollInterval = time.Millisecond
	require.NoError(t, c.Start())
	t.Cleanup(func() { require.NoError(t, c.Stop()) })
	require.Eventually(t, func() bool {
		pending, err := c.HasPending(t.Context(), intent.Operations[0])
		return err == nil && !pending
	}, time.Second, time.Millisecond)
}

type uncertainCommitStore struct {
	store.Store
	uncertain bool
}

type unavailableNonceStore struct{ store.Store }

func (s unavailableNonceStore) ListTxNoncesFrom(context.Context, string, string, uint64) ([]uint64, error) {
	return nil, errors.New("journal unavailable")
}

func TestJournalReadFailureDoesNotReuseMinedNonce(t *testing.T) {
	c, rpc, db := newManager(t)
	require.NoError(t, c.submit(t.Context(), packetIntent(t, db, 1)))
	second := packetIntent(t, db, 2)
	c.db = unavailableNonceStore{Store: db}
	c.reconciler.db = c.db
	require.ErrorContains(t, c.submit(t.Context(), second), "journal unavailable")
	require.Len(t, rpc.sent, 1)
	c.db = db
	c.reconciler.db = c.db
	require.NoError(t, c.submit(t.Context(), second))
	require.Equal(t, uint64(1), rpc.sent[1].Nonce())
}

func (s *uncertainCommitStore) Transact(ctx context.Context, f func(store.Repository) error) error {
	err := s.Store.Transact(ctx, f)
	if err == nil && s.uncertain {
		s.uncertain = false
		return errors.New("lost commit response")
	}
	return err
}

func TestUncertainDatabaseCommitIsRestoredBeforeAllocation(t *testing.T) {
	c, rpc, db := newManager(t)
	first := packetIntent(t, db, 1)
	second := packetIntent(t, db, 2)
	c.db = &uncertainCommitStore{Store: db, uncertain: true}
	c.reconciler.db = c.db
	err := c.submit(t.Context(), first)
	require.ErrorContains(t, err, "lost commit response")
	require.Empty(t, rpc.sent)
	err = c.submit(t.Context(), second)
	require.NoError(t, err)
	require.Equal(t, uint64(1), rpc.sent[0].Nonce())
	c.reconciler.reconcile(t.Context())
	require.Len(t, rpc.sent, 2)
	require.Equal(t, uint64(0), rpc.sent[1].Nonce())
}

func TestResolutionUsesReportedCanonicalBlockHash(t *testing.T) {
	c, rpc, db := newManager(t)
	rpc.reportedHash = common.HexToHash("0xbe5a")
	intent := packetIntent(t, db, 1)
	err := c.submit(t.Context(), intent)
	require.NoError(t, err)
	tx := rpc.sent[0]
	rpc.mined = 1
	rpc.receipts[tx.Hash()] = &types.Receipt{
		TxHash:      tx.Hash(),
		BlockHash:   rpc.reportedHash,
		BlockNumber: rpc.head.Number,
		Status:      types.ReceiptStatusSuccessful,
	}
	c.reconciler.reconcile(t.Context())
	status, err := c.HasPending(t.Context(), intent.Operations[0])
	require.NoError(t, err)
	require.False(t, status)
}

func TestReorgBeforeResolutionResumesBroadcast(t *testing.T) {
	c, rpc, db := newManager(t)
	intent := packetIntent(t, db, 1)
	require.NoError(t, c.submit(t.Context(), intent))
	tx := rpc.sent[0]
	rpc.mined = 1
	rpc.receiptErr = errors.New("receipt unavailable")
	c.reconciler.reconcile(t.Context())
	pending, err := c.HasPending(t.Context(), intent.Operations[0])
	require.NoError(t, err)
	require.True(t, pending)
	require.Len(t, rpc.sent, 1)

	rpc.mined = 0
	rpc.receiptErr = nil
	rpc.reportedHash = common.HexToHash("0xf00d")
	require.NoError(
		t,
		db.SetTxBroadcast(t.Context(), chainIDEth, tx.Hash().Hex(), time.Now().Add(-2*c.opts.PollInterval)),
	)
	c.reconciler.reconcile(t.Context())
	require.Len(t, rpc.sent, 2)
	require.Equal(t, tx.Hash(), rpc.sent[1].Hash())
	require.NoError(t, c.submit(t.Context(), packetIntent(t, db, 2)))
	require.Equal(t, uint64(1), rpc.sent[2].Nonce())

	rpc.mined = 1
	c.reconciler.reconcile(t.Context())
	pending, err = c.HasPending(t.Context(), intent.Operations[0])
	require.NoError(t, err)
	require.False(t, pending)
}

func TestReorgAfterResolutionRestoresAttempts(t *testing.T) {
	for _, status := range []string{store.TxConfirmed, store.TxFailed, store.TxConsumed} {
		t.Run(status, func(t *testing.T) {
			c, rpc, db := newManager(t)
			c.opts.RepriceInterval = time.Nanosecond
			intent := packetIntent(t, db, 1)
			require.NoError(t, c.submit(t.Context(), intent))
			first := rpc.sent[0]
			c.reconciler.reconcile(t.Context())
			require.Len(t, rpc.sent, 2)
			latest := rpc.sent[1]
			rpc.mined = 1
			if status != store.TxConsumed {
				receiptStatus := types.ReceiptStatusSuccessful
				if status == store.TxFailed {
					receiptStatus = types.ReceiptStatusFailed
				}
				rpc.receipts[first.Hash()] = &types.Receipt{
					TxHash: first.Hash(), BlockHash: rpc.head.Hash(), BlockNumber: rpc.head.Number,
					Status: receiptStatus,
				}
			}
			c.reconciler.reconcile(t.Context())
			require.Empty(t, pendingAttempts(t, c))
			resolved, err := db.GetTxAttempt(t.Context(), chainIDEth, first.Hash().Hex())
			require.NoError(t, err)
			require.Equal(t, status, resolved.Status)

			restarted, err := New(chainIDEth, rpc, c.signer, db, c.opts)
			require.NoError(t, err)
			restarted.ctx = t.Context()
			restarted.reconciler.reconcile(t.Context())
			require.Empty(t, pendingAttempts(t, restarted))
			require.Len(t, rpc.sent, 2, "resolved transactions stay idle while their nonce remains mined")

			rpc.mined = 0
			rpc.head.Extra = []byte("reorg")
			delete(rpc.receipts, first.Hash())
			restarted.reconciler.reconcile(t.Context())
			require.Len(t, pendingAttempts(t, restarted), 2)
			require.Len(t, rpc.sent, 3)
			require.Equal(t, latest.Hash(), rpc.sent[2].Hash(), "rebroadcast the latest signed attempt unchanged")
			pending, err := restarted.HasPending(t.Context(), intent.Operations[0])
			require.NoError(t, err)
			require.True(t, pending)

			restarted.reconciler.reconcile(t.Context())
			require.Len(t, rpc.sent, 4)
			replacement := rpc.sent[3]
			require.Equal(t, latest.Nonce(), replacement.Nonce())
			require.Equal(t, latest.To(), replacement.To())
			require.Equal(t, latest.Data(), replacement.Data())
			require.Positive(t, replacement.GasFeeCap().Cmp(latest.GasFeeCap()))
			require.Positive(t, replacement.GasTipCap().Cmp(latest.GasTipCap()))
			rpc.mined = 1
			rpc.receipts[replacement.Hash()] = &types.Receipt{
				TxHash: replacement.Hash(), BlockHash: rpc.head.Hash(), BlockNumber: rpc.head.Number,
				Status: types.ReceiptStatusSuccessful,
			}
			restarted.reconciler.reconcile(t.Context())
			require.Empty(t, pendingAttempts(t, restarted))
			resolved, err = db.GetTxAttempt(t.Context(), chainIDEth, replacement.Hash().Hex())
			require.NoError(t, err)
			require.Equal(t, store.TxConfirmed, resolved.Status)
			resolved, err = db.GetTxAttempt(t.Context(), chainIDEth, first.Hash().Hex())
			require.NoError(t, err)
			require.Equal(t, store.TxReplaced, resolved.Status)
		})
	}
}

func TestLaggingRPCDoesNotReopenResolvedAttempts(t *testing.T) {
	for _, status := range []string{store.TxConfirmed, store.TxFailed, store.TxConsumed} {
		t.Run(status, func(t *testing.T) {
			c, rpc, db := newManager(t)
			c.opts.RepriceInterval = time.Nanosecond
			require.NoError(t, c.submit(t.Context(), packetIntent(t, db, 1)))
			first := rpc.sent[0]
			c.reconciler.reconcile(t.Context())
			rpc.mined = 1
			if status != store.TxConsumed {
				receiptStatus := types.ReceiptStatusSuccessful
				if status == store.TxFailed {
					receiptStatus = types.ReceiptStatusFailed
				}
				rpc.receipts[first.Hash()] = &types.Receipt{
					TxHash: first.Hash(), BlockHash: rpc.head.Hash(), BlockNumber: new(big.Int).Set(rpc.head.Number),
					Status: receiptStatus,
				}
			}
			c.reconciler.reconcile(t.Context())
			require.Empty(t, pendingAttempts(t, c))

			restarted, err := New(chainIDEth, rpc, c.signer, db, c.opts)
			require.NoError(t, err)
			restarted.ctx = t.Context()
			rpc.mined = 0
			rpc.head.Number = big.NewInt(9)
			restarted.reconciler.reconcile(t.Context())
			require.Empty(t, pendingAttempts(t, restarted))
			require.Len(t, rpc.sent, 2, "no rebroadcast or repricing of resolved attempts")
			attempt, err := db.GetTxAttempt(t.Context(), chainIDEth, first.Hash().Hex())
			require.NoError(t, err)
			require.Equal(t, status, attempt.Status)
		})
	}
}

func TestReorgDuringResolutionKeepsAttemptPending(t *testing.T) {
	c, rpc, db := newManager(t)
	require.NoError(t, c.submit(t.Context(), packetIntent(t, db, 1)))
	rpc.mined = 1
	rpc.afterNonce = func() {
		rpc.head.Extra = []byte("reorg")
		rpc.mined = 0
	}
	c.reconciler.reconcile(t.Context())
	require.Len(t, pendingAttempts(t, c), 1)
	resolutions, err := db.ListTxResolutionsFrom(t.Context(), chainIDEth, c.address.Hex(), 0)
	require.NoError(t, err)
	require.Empty(t, resolutions)
}

func TestReorgDuringReopeningRecoversOnNextPass(t *testing.T) {
	c, rpc, db := newManager(t)
	require.NoError(t, c.submit(t.Context(), packetIntent(t, db, 1)))
	rpc.mined = 1
	c.reconciler.reconcile(t.Context())
	require.Empty(t, pendingAttempts(t, c))
	rpc.mined = 0
	rpc.head.Extra = []byte("first reorg")
	rpc.afterNonce = func() {
		rpc.head.Extra = []byte("second reorg")
		rpc.mined = 1
	}
	c.reconciler.reconcile(t.Context())
	require.Len(t, pendingAttempts(t, c), 1)
	require.Len(t, rpc.sent, 2)
	rpc.afterNonce = nil
	c.reconciler.reconcile(t.Context())
	require.Empty(t, pendingAttempts(t, c))
	require.Len(t, rpc.sent, 2)
}

func TestAllocationPreservesResolvedNonceAfterReorg(t *testing.T) {
	c, rpc, db := newManager(t)
	require.NoError(t, c.submit(t.Context(), packetIntent(t, db, 1)))
	first := rpc.sent[0]
	rpc.mined = 1
	c.reconciler.reconcile(t.Context())
	require.Empty(t, pendingAttempts(t, c))

	rpc.mined = 0
	rpc.head.Extra = []byte("reorg")
	require.NoError(t, c.submit(t.Context(), packetIntent(t, db, 2)))
	require.Equal(
		t,
		uint64(1),
		rpc.sent[1].Nonce(),
		"allocation must reserve resolved nonces before polling reopens them",
	)
	c.reconciler.reconcile(t.Context())
	require.Len(t, pendingAttempts(t, c), 2)
	require.Len(t, rpc.sent, 3)
	require.Equal(t, first.Hash(), rpc.sent[2].Hash())
}

func TestRestartFillsBlockingGapAtCapacity(t *testing.T) {
	c, rpc, db := newManager(t)
	c.opts.MaxInFlight = 1
	intent := packetIntent(t, db, 1)
	higher := saveIntentAtNonce(t, c, intent, 1)
	restarted, err := New(chainIDEth, rpc, c.signer, db, c.opts)
	require.NoError(t, err)
	restarted.ctx = t.Context()
	restarted.reconciler.reconcile(t.Context())
	require.Len(t, rpc.sent, 1)
	require.Equal(t, higher.Hash(), rpc.sent[0].Hash())
	require.NoError(t, restarted.submit(t.Context(), packetIntent(t, db, 2)))
	require.Len(t, rpc.sent, 2)
	require.Equal(t, uint64(0), rpc.sent[1].Nonce())
	require.Equal(t, common.HexToAddress(intent.To), *rpc.sent[1].To())
	require.Equal(t, intent.Data, rpc.sent[1].Data())
	require.ErrorIs(t, restarted.submit(t.Context(), packetIntent(t, db, 3)), ErrBusy)
}

func TestAllocationFillsInteriorGapWithoutReusingOwnedNonces(t *testing.T) {
	c, rpc, db := newManager(t)
	require.NoError(t, c.submit(t.Context(), packetIntent(t, db, 1)))
	higher := saveIntentAtNonce(t, c, packetIntent(t, db, 2), 2)
	require.NoError(t, c.submit(t.Context(), packetIntent(t, db, 3)))
	require.Equal(t, uint64(1), rpc.sent[1].Nonce())
	require.NoError(t, c.submit(t.Context(), packetIntent(t, db, 4)))
	require.Equal(t, uint64(3), rpc.sent[2].Nonce())
	saved, err := db.GetTxAttempt(t.Context(), chainIDEth, higher.Hash().Hex())
	require.NoError(t, err)
	require.Equal(t, uint64(2), saved.Nonce)
	require.Equal(t, store.TxPending, saved.Status)
}

func TestMinedRevertAllowsPacketReconciliation(t *testing.T) {
	c, rpc, db := newManager(t)
	intent := packetIntent(t, db, 1)
	err := c.submit(t.Context(), intent)
	require.NoError(t, err)
	c.opts.RepriceInterval = time.Nanosecond
	c.reconciler.reconcile(t.Context())
	require.Len(t, rpc.sent, 2)
	tx := rpc.sent[1]
	rpc.mined = 1
	rpc.receipts[tx.Hash()] = &types.Receipt{
		TxHash:      tx.Hash(),
		BlockHash:   rpc.head.Hash(),
		BlockNumber: rpc.head.Number,
		Status:      types.ReceiptStatusFailed,
	}
	c.reconciler.reconcile(t.Context())
	status, err := c.HasPending(t.Context(), intent.Operations[0])
	require.NoError(t, err)
	require.False(t, status)
	require.Empty(t, pendingAttempts(t, c))
	loser, err := db.GetTxAttempt(t.Context(), chainIDEth, rpc.sent[0].Hash().Hex())
	require.NoError(t, err)
	require.Equal(t, store.TxReplaced, loser.Status)
	winner, err := db.GetTxAttempt(t.Context(), chainIDEth, tx.Hash().Hex())
	require.NoError(t, err)
	require.Equal(t, store.TxFailed, winner.Status)
}

type malformedSigner struct{ signer.Signer }

func (s malformedSigner) Sign(context.Context, []byte) ([]byte, error) { return []byte{1}, nil }

func TestMalformedSignerResponseDoesNotPanicOrBroadcast(t *testing.T) {
	c, rpc, db := newManager(t)
	c.signer = malformedSigner{Signer: c.signer}
	c.reconciler.signer = c.signer
	err := c.submit(t.Context(), packetIntent(t, db, 1))
	require.ErrorContains(t, err, "invalid signature length")
	require.Empty(t, rpc.sent)
}

func TestPacketStatusSurvivesRestartWithoutPacketHash(t *testing.T) {
	c, rpc, db := newManager(t)
	intent := packetIntent(t, db, 1)
	require.NoError(t, c.submit(t.Context(), intent))
	saved, err := db.ListPacketsBySourceTx(t.Context(), "2", "0xsend")
	require.NoError(t, err)
	require.Nil(t, saved[0].RecvTxHash)
	restarted, err := New(chainIDEth, rpc, c.signer, db, c.opts)
	require.NoError(t, err)
	require.NoError(t, restarted.Start())
	t.Cleanup(func() { require.NoError(t, restarted.Stop()) })
	status, err := restarted.HasPending(t.Context(), intent.Operations[0])
	require.NoError(t, err)
	require.True(t, status)
	require.NoError(t, restarted.Submit(t.Context(), intent))
	timeout := intent.Operations[0]
	timeout.Type = v2.OperationTypeTimeout
	status, err = restarted.HasPending(t.Context(), timeout)
	require.NoError(t, err)
	require.False(t, status)
}

func TestStatusOnlyTracksJournaledOperations(t *testing.T) {
	c, rpc, db := newManager(t)
	intent := packetIntent(t, db, 1)
	require.NoError(t, db.UpdatePacketRecvTx(t.Context(), store.PacketKey{
		SourceChainID: "2", SourceClientID: "source", Sequence: 1,
	}, store.PacketTx{Hash: "0xreceived", Time: time.Now(), RelayerAddress: "another-relayer"}))
	rpc.errorRead = errors.New("RPC offline")
	status, err := c.HasPending(t.Context(), intent.Operations[0])
	require.NoError(t, err)
	require.False(t, status)
}

func TestNewSubmissionAfterResolutionIsPending(t *testing.T) {
	c, rpc, db := newManager(t)
	intent := packetIntent(t, db, 1)
	require.NoError(t, c.submit(t.Context(), intent))
	first := rpc.sent[0]
	rpc.mined = 1
	rpc.receipts[first.Hash()] = &types.Receipt{
		TxHash:      first.Hash(),
		BlockHash:   rpc.head.Hash(),
		BlockNumber: rpc.head.Number,
		Status:      types.ReceiptStatusFailed,
	}
	c.reconciler.reconcile(t.Context())
	status, err := c.HasPending(t.Context(), intent.Operations[0])
	require.NoError(t, err)
	require.False(t, status)
	require.NoError(t, c.submit(t.Context(), intent))
	status, err = c.HasPending(t.Context(), intent.Operations[0])
	require.NoError(t, err)
	require.True(t, status)
	require.Equal(t, uint64(1), rpc.sent[1].Nonce())
	old, err := db.GetTxAttempt(t.Context(), chainIDEth, first.Hash().Hex())
	require.NoError(t, err)
	require.Equal(t, store.TxFailed, old.Status)
}

type failedResolutionStore struct{ store.Store }

func (s failedResolutionStore) ResolveTxNonce(context.Context, store.TxOutcome) error {
	return errors.New("journal unavailable")
}

func TestResolutionFailureIsRetried(t *testing.T) {
	c, rpc, db := newManager(t)
	intent := packetIntent(t, db, 1)
	require.NoError(t, c.submit(t.Context(), intent))
	rpc.mined = 1
	c.db = failedResolutionStore{Store: db}
	c.reconciler.db = c.db
	c.reconciler.reconcile(t.Context())
	pending, err := c.HasPending(t.Context(), intent.Operations[0])
	require.NoError(t, err)
	require.True(t, pending)
	c.db = db
	c.reconciler.db = c.db
	c.reconciler.reconcile(t.Context())
	pending, err = c.HasPending(t.Context(), intent.Operations[0])
	require.NoError(t, err)
	require.False(t, pending)
}

type stalledReceiptRPC struct {
	ETHClient
	started chan struct{}
	once    sync.Once
}

func (r *stalledReceiptRPC) TransactionReceipt(ctx context.Context, _ common.Hash) (*types.Receipt, error) {
	r.once.Do(func() { close(r.started) })
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestSlowPollingDoesNotBlockSubmission(t *testing.T) {
	c, rpc, db := newManager(t)
	stalled := &stalledReceiptRPC{ETHClient: rpc, started: make(chan struct{})}
	c.reconciler.eth = stalled
	first, second := packetIntent(t, db, 1), packetIntent(t, db, 2)
	require.NoError(t, c.submit(t.Context(), first))
	rpc.mined = 1
	started := make(chan error, 1)
	go func() { started <- c.Start() }()
	t.Cleanup(func() { require.NoError(t, c.Stop()) })
	select {
	case err := <-started:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("startup blocked on recovery")
	}
	select {
	case <-stalled.started:
	case <-time.After(time.Second):
		t.Fatal("polling did not start")
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	require.NoError(t, c.Submit(ctx, second))
	require.Equal(t, 2, rpc.sendCount())
	attempts := pendingAttempts(t, c)
	require.Len(t, attempts, 2)
	require.Equal(t, uint64(0), attempts[0].Nonce)
	require.Equal(t, uint64(1), attempts[1].Nonce)
}

type pausedNonceReadStore struct {
	store.Store
	started chan struct{}
	release chan struct{}
}

func (s pausedNonceReadStore) ListTxNoncesFrom(
	ctx context.Context,
	chainID, sender string,
	minedNonce uint64,
) ([]uint64, error) {
	select {
	case s.started <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case <-s.release:
		return s.Store.ListTxNoncesFrom(ctx, chainID, sender, minedNonce)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestResolutionDuringAllocationPreservesNonceOwnership(t *testing.T) {
	c, rpc, db := newManager(t)
	first, second := packetIntent(t, db, 1), packetIntent(t, db, 2)
	second.Data = []byte{0xbe, 0xef}
	require.NoError(t, c.submit(t.Context(), first))
	paused := pausedNonceReadStore{Store: db, started: make(chan struct{}, 2), release: make(chan struct{})}
	c.db = paused
	c.reconciler.db = c.db
	result := make(chan error, 1)
	go func() { result <- c.submit(t.Context(), second) }()
	select {
	case <-paused.started:
	case <-time.After(time.Second):
		t.Fatal("submission did not read mined nonce")
	}
	rpc.mu.Lock()
	rpc.mined = 1
	rpc.mu.Unlock()
	c.reconciler.reconcile(t.Context())
	require.Empty(t, pendingAttempts(t, c))
	close(paused.release)
	require.NoError(t, <-result)
	attempts := pendingAttempts(t, c)
	require.Len(t, attempts, 1)
	require.Equal(t, uint64(1), attempts[0].Nonce, "resolved nonces remain owned after a stale mined-nonce read")
}

func TestSubmitRejectsContractSimulationRevert(t *testing.T) {
	submitter, eth, _ := newTestTxSubmitter(t, ChainOptions{})
	to := common.HexToAddress(toAddress)
	data := []byte{0xde, 0xad, 0xbe, 0xef}
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
	eth.EXPECT().HeaderByNumber(mock.Anything, (*big.Int)(nil)).
		Return(&types.Header{BaseFee: big.NewInt(100), GasLimit: 30000000}, nil).Once()
	eth.EXPECT().SuggestGasTipCap(mock.Anything).Return(big.NewInt(10), nil).Once()
	eth.EXPECT().PendingCodeAt(mock.Anything, common.HexToAddress(toAddress)).Return([]byte{1}, nil).Once()
	eth.EXPECT().EstimateGas(mock.Anything, ethereum.CallMsg{From: submitter.address, To: &to, Data: data}).
		Return(0, revert).Once()
	result, err := submitter.Submit(t.Context(), v2.TxIntent{To: toAddress, Data: data})
	require.Nil(t, result)
	require.ErrorIs(t, err, revert)
	require.EqualError(t, err, "creating tx: estimating gas: ConsensusStateExpired(100, 121, 20): execution reverted")
	eth.AssertNotCalled(t, "PendingNonceAt", mock.Anything, mock.Anything)
	eth.AssertNotCalled(t, "SendTransaction", mock.Anything, mock.Anything)
}
