// SPDX-License-Identifier: Apache-2.0

package pipeline

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	"github.com/cosmos/ibc/cli/internal/chains"
	"github.com/cosmos/ibc/cli/internal/relay/processors"
	"github.com/cosmos/ibc/cli/internal/relay/prover"
	"github.com/cosmos/ibc/cli/internal/relay/txbuilder"
	"github.com/cosmos/ibc/cli/internal/store"
	"github.com/cosmos/ibc/cli/internal/tests/mocks"
	"github.com/cosmos/ibc/cli/internal/txsubmitter"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

const (
	recvTxHash    = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	ackTxHash     = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	timeoutTxHash = "0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
)

var testRoute = processors.Route{
	SourceChainID:       "1",
	SourceClientID:      "base-0",
	DestinationChainID:  "8453",
	DestinationClientID: "ethereum-0",
}

type pipelineEnv struct {
	store     *store.SqliteDB
	srcClient *mocks.MockClient
	dstClient *mocks.MockClient
	// dstProver/dstTxBuilder are resolved for the destination client/chain,
	// used by recv delivery; srcProver/srcTxBuilder are resolved for the
	// source client/chain, used by ack and timeout delivery.
	dstProver      *mocks.MockProver
	srcProver      *mocks.MockProver
	dstTxBuilder   *mocks.MockTxBuilder
	srcTxBuilder   *mocks.MockTxBuilder
	srcTxSubmitter *mocks.MockTxSubmitter
	dstTxSubmitter *mocks.MockTxSubmitter
}

type staticChains map[string]chains.Client

func (s staticChains) Get(chainID string) (chains.Client, bool) {
	client, ok := s[chainID]
	return client, ok
}

type staticProvers map[string]prover.Prover

func (s staticProvers) Get(chainID, clientID string) (prover.Prover, bool) {
	gen, ok := s[prover.Key(chainID, clientID)]
	return gen, ok
}

type staticTxBuilders map[string]txbuilder.TxBuilder

func (s staticTxBuilders) Get(chainID string) (txbuilder.TxBuilder, bool) {
	builder, ok := s[chainID]
	return builder, ok
}

func newPipelineEnv(t *testing.T) (*pipelineEnv, Deps) {
	t.Helper()

	db, err := store.NewSqliteInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.MigrateUp()
	require.NoError(t, err)

	env := &pipelineEnv{
		store:          db,
		srcClient:      mocks.NewMockClient(t),
		dstClient:      mocks.NewMockClient(t),
		dstProver:      mocks.NewMockProver(t),
		srcProver:      mocks.NewMockProver(t),
		dstTxBuilder:   mocks.NewMockTxBuilder(t),
		srcTxBuilder:   mocks.NewMockTxBuilder(t),
		srcTxSubmitter: mocks.NewMockTxSubmitter(t),
		dstTxSubmitter: mocks.NewMockTxSubmitter(t),
	}
	env.srcClient.EXPECT().ChainID().Return(testRoute.SourceChainID).Maybe()
	env.dstClient.EXPECT().ChainID().Return(testRoute.DestinationChainID).Maybe()

	deps := Deps{
		Storage: db,
		Chains:  staticChains{testRoute.SourceChainID: env.srcClient, testRoute.DestinationChainID: env.dstClient},
		Provers: staticProvers{
			prover.Key(testRoute.DestinationChainID, testRoute.DestinationClientID): env.dstProver,
			prover.Key(testRoute.SourceChainID, testRoute.SourceClientID):           env.srcProver,
		},
		TxBuilders: staticTxBuilders{
			testRoute.DestinationChainID: env.dstTxBuilder,
			testRoute.SourceChainID:      env.srcTxBuilder,
		},
		TxSubmitters: staticTxSubmitters{
			testRoute.SourceChainID:      env.srcTxSubmitter,
			testRoute.DestinationChainID: env.dstTxSubmitter,
		},
	}

	return env, deps
}

// mockRelay wires up one batch delivery's worth of expectations across the
// packet event, proof generation, and tx building stages, mirroring what the
// real batch processors call in sequence. client is the mocks.MockClient
// for whichever chain the batch reads packet events from.
func mockRelay(
	client *mocks.MockClient,
	mockProver *mocks.MockProver,
	txBuilder *mocks.MockTxBuilder,
	events []v2.PacketEvent,
	to string,
) {
	const height = uint64(100)

	client.EXPECT().TxPacketEvents(mock.Anything, mock.Anything).Return(events, nil).Once()
	mockProver.EXPECT().LatestProvableHeight(mock.Anything).Return(height, time.Now(), nil).Maybe()
	mockProver.EXPECT().ClientUpdatePayloads(mock.Anything, height).Return([][]byte{{0x01}}, nil).Once()
	mockProver.EXPECT().PacketProofs(mock.Anything, height, mock.Anything, mock.Anything, mock.Anything).
		Return(make([][]byte, len(events)), nil).Once()
	txBuilder.EXPECT().BuildRelayTxs(mock.Anything, mock.Anything).
		Return([]v2.RelayTx{{To: common.HexToAddress(to).Bytes(), Data: []byte{0xca, 0x11}}}, nil).Once()
}

func sendPacketEvent(sequence uint64) v2.PacketEvent {
	return v2.PacketEvent{
		Height: 100,
		Kind:   v2.KindSendPacket,
		Packet: channeltypesv2.Packet{
			Sequence:          sequence,
			SourceClient:      testRoute.SourceClientID,
			DestinationClient: testRoute.DestinationClientID,
		},
	}
}

func writeAckEvent(sequence uint64) v2.PacketEvent {
	event := sendPacketEvent(sequence)
	event.Kind = v2.KindWriteAck
	event.Acks = [][]byte{{0xac}}

	return event
}

func (env *pipelineEnv) createPacket(t *testing.T, timeout time.Time) *processors.Transfer {
	t.Helper()

	ctx := context.Background()
	input := store.UpsertPacket{
		Status:                    store.RelayStatusPending,
		SourceChainID:             testRoute.SourceChainID,
		DestinationChainID:        testRoute.DestinationChainID,
		SourceTxHash:              "0x60016c34c02278856c81a41ce857ac4bb837a2f4a13c95207e08cbc9e8f2b706",
		SourceTxTime:              time.Now().UTC(),
		PacketSequenceNumber:      42,
		PacketSourceClientID:      testRoute.SourceClientID,
		PacketDestinationClientID: testRoute.DestinationClientID,
		PacketTimeoutTimestamp:    timeout,
	}
	require.NoError(t, env.store.UpsertPacket(ctx, input))

	packets, err := env.store.ListPacketsBySourceTx(ctx, input.SourceChainID, input.SourceTxHash)
	require.NoError(t, err)
	require.Len(t, packets, 1)

	return processors.NewTransfer(packets[0], slog.Default())
}

func (env *pipelineEnv) storedPacket(t *testing.T, tr *processors.Transfer) store.Packet {
	t.Helper()

	packets, err := env.store.ListPacketsBySourceTx(context.Background(), tr.SourceChainID, tr.SourceTxHash)
	require.NoError(t, err)
	require.Len(t, packets, 1)

	return packets[0]
}

func fastOpts() Options {
	return Options{
		SourceSignerAlias:   "test-signer",
		DestSignerAlias:     "test-signer",
		RecvBatchSize:       1,
		RecvBatchTimeout:    50 * time.Millisecond,
		AckBatchSize:        1,
		AckBatchTimeout:     50 * time.Millisecond,
		TimeoutBatchSize:    1,
		TimeoutBatchTimeout: 50 * time.Millisecond,
	}
}

func runPipeline(t *testing.T, deps Deps, opts Options, tr *processors.Transfer) *processors.Transfer {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p, err := NewPipeline(ctx, slog.Default(), deps, testRoute, opts)
	require.NoError(t, err)
	require.True(t, p.Push(ctx, tr))

	type pollResult struct {
		transfer *processors.Transfer
		err      error
	}

	done := make(chan pollResult, 1)
	go func() {
		out, err := p.Poll()
		done <- pollResult{transfer: out, err: err}
	}()

	select {
	case result := <-done:
		p.Close()
		require.NoError(t, result.err)

		return result.transfer
	case <-time.After(15 * time.Second):
		require.FailNow(t, "pipeline did not emit the transfer in time")

		return nil
	}
}

func TestPipelineAcknowledgement(t *testing.T) {
	testAcknowledgementLifecycle(t, v2.WriteAckStatusSuccess, store.RelayStatusCompleteWithAck)
}

func TestPipelineErrorAcknowledgement(t *testing.T) {
	testAcknowledgementLifecycle(t, v2.WriteAckStatusError, store.RelayStatusCompleteWithWriteAckError)
}

func testAcknowledgementLifecycle(t *testing.T, ack v2.WriteAckStatus, terminal store.RelayStatus) {
	t.Helper()
	env, deps := newPipelineEnv(t)
	tr := env.createPacket(t, time.Now().Add(time.Hour))
	var received, settled atomic.Bool
	env.dstClient.EXPECT().IsPacketReceived(mock.Anything, testRoute.DestinationClientID, uint64(42)).
		RunAndReturn(func(context.Context, string, uint64) (bool, error) { return received.Load(), nil })
	env.dstClient.EXPECT().FindRecvTx(mock.Anything, testRoute.DestinationClientID, uint64(42)).
		Return(&v2.Tx{Hash: recvTxHash, Timestamp: time.Now(), RelayerAddress: "0xrelayer"}, nil)
	env.srcClient.EXPECT().IsPacketCommitted(mock.Anything, testRoute.SourceClientID, uint64(42)).
		RunAndReturn(func(context.Context, string, uint64) (bool, error) { return !settled.Load(), nil })
	env.srcClient.EXPECT().FindAckTx(mock.Anything, testRoute.SourceClientID, uint64(42)).
		Return(&v2.Tx{Hash: ackTxHash, Timestamp: time.Now(), RelayerAddress: "0xrelayer"}, nil)
	env.srcClient.EXPECT().
		FindTimeoutTx(mock.Anything, testRoute.SourceClientID, uint64(42)).
		Return(nil, v2.ErrTxNotFound)
	env.dstProver.EXPECT().LatestProvableHeight(mock.Anything).Return(uint64(100), time.Now(), nil).Once()
	env.srcClient.EXPECT().TxHeight(mock.Anything, mock.Anything).Return(uint64(100), nil).Once()
	mockRelay(env.srcClient, env.dstProver, env.dstTxBuilder, []v2.PacketEvent{sendPacketEvent(42)}, "0xrouter")
	env.dstClient.EXPECT().WaitForChain(mock.Anything).Return(nil).Once()
	env.dstTxSubmitter.EXPECT().
		HasPending(mock.Anything, mock.MatchedBy(func(op v2.OperationMetadata) bool { return op.Type == v2.OperationTypeRecv })).
		Return(false, nil)
	env.dstTxSubmitter.EXPECT().Submit(mock.Anything, mock.Anything).
		Run(func(context.Context, v2.TxIntent) { received.Store(true) }).Return(nil).Once()
	env.dstClient.EXPECT().
		PacketWriteAckStatus(mock.Anything, recvTxHash, uint64(42), testRoute.SourceClientID, testRoute.DestinationClientID).
		Return(ack, nil).
		Once()
	env.srcProver.EXPECT().LatestProvableHeight(mock.Anything).Return(uint64(100), time.Now(), nil)
	env.dstClient.EXPECT().TxHeight(mock.Anything, mock.Anything).Return(uint64(100), nil)
	mockRelay(env.dstClient, env.srcProver, env.srcTxBuilder, []v2.PacketEvent{writeAckEvent(42)}, "0xrouter")
	env.srcClient.EXPECT().WaitForChain(mock.Anything).Return(nil).Once()
	env.srcTxSubmitter.EXPECT().
		HasPending(mock.Anything, mock.MatchedBy(func(op v2.OperationMetadata) bool { return op.Type == v2.OperationTypeAck })).
		Return(false, nil)
	env.srcTxSubmitter.EXPECT().Submit(mock.Anything, mock.Anything).
		Run(func(context.Context, v2.TxIntent) { settled.Store(true) }).Return(nil).Once()

	out := runPipeline(t, deps, fastOpts(), tr)
	require.ErrorIs(t, out.ProcessingError, processors.ErrSubmissionPending)
	saved := env.storedPacket(t, tr)
	require.Nil(t, saved.RecvTxHash, "acceptance must not populate observed transaction fields")

	out = runPipeline(t, deps, fastOpts(), processors.NewTransfer(saved, slog.Default()))
	require.ErrorIs(t, out.ProcessingError, processors.ErrSubmissionPending)
	saved = env.storedPacket(t, tr)
	require.Equal(t, recvTxHash, *saved.RecvTxHash)
	require.Nil(t, saved.AckTxHash)

	out = runPipeline(t, deps, fastOpts(), processors.NewTransfer(saved, slog.Default()))
	require.NoError(t, out.ProcessingError)
	require.Equal(t, terminal, out.Status)
	saved = env.storedPacket(t, tr)
	require.Equal(t, terminal, saved.Status)
	require.Equal(t, ackTxHash, *saved.AckTxHash)
}

func TestPipelineTimeout(t *testing.T) {
	env, deps := newPipelineEnv(t)
	tr := env.createPacket(t, time.Now().Add(-time.Hour))
	var settled atomic.Bool
	env.dstClient.EXPECT().IsPacketReceived(mock.Anything, testRoute.DestinationClientID, uint64(42)).Return(false, nil)
	env.srcClient.EXPECT().IsPacketCommitted(mock.Anything, testRoute.SourceClientID, uint64(42)).
		RunAndReturn(func(context.Context, string, uint64) (bool, error) { return !settled.Load(), nil })
	env.srcClient.EXPECT().FindAckTx(mock.Anything, testRoute.SourceClientID, uint64(42)).Return(nil, v2.ErrTxNotFound)
	env.srcClient.EXPECT().FindTimeoutTx(mock.Anything, testRoute.SourceClientID, uint64(42)).
		Return(&v2.Tx{Hash: timeoutTxHash, Timestamp: time.Now(), RelayerAddress: "0xrelayer"}, nil)
	env.dstProver.EXPECT().LatestProvableHeight(mock.Anything).Return(uint64(100), time.Now(), nil)
	env.srcClient.EXPECT().TxHeight(mock.Anything, mock.Anything).Return(uint64(100), nil)
	env.srcProver.EXPECT().LatestProvableHeight(mock.Anything).Return(uint64(100), time.Now(), nil)
	event := sendPacketEvent(42)
	event.Height = 150
	mockRelay(env.srcClient, env.srcProver, env.srcTxBuilder, []v2.PacketEvent{event}, "0xrouter")
	env.srcClient.EXPECT().WaitForChain(mock.Anything).Return(nil).Once()
	env.srcTxSubmitter.EXPECT().
		HasPending(mock.Anything, mock.MatchedBy(func(op v2.OperationMetadata) bool { return op.Type == v2.OperationTypeTimeout })).
		Return(false, nil)
	env.srcTxSubmitter.EXPECT().Submit(mock.Anything, mock.Anything).
		Run(func(context.Context, v2.TxIntent) { settled.Store(true) }).Return(nil).Once()
	out := runPipeline(t, deps, fastOpts(), tr)
	require.ErrorIs(t, out.ProcessingError, processors.ErrSubmissionPending)
	saved := env.storedPacket(t, tr)
	require.Nil(t, saved.TimeoutTxHash)
	out = runPipeline(t, deps, fastOpts(), processors.NewTransfer(saved, slog.Default()))
	require.NoError(t, out.ProcessingError)
	require.Equal(t, store.RelayStatusCompleteWithTimeout, out.Status)
	saved = env.storedPacket(t, tr)
	require.Equal(t, timeoutTxHash, *saved.TimeoutTxHash)
}

func TestPipelineUnfinalizedSend(t *testing.T) {
	t.Run("sendNotFinalizedPoisonsForRun", func(t *testing.T) {
		env, deps := newPipelineEnv(t)
		tr := env.createPacket(t, time.Now().Add(time.Hour))

		env.dstClient.EXPECT().
			IsPacketReceived(mock.Anything, testRoute.DestinationClientID, uint64(42)).
			Return(false, nil).
			Once()
		env.srcClient.EXPECT().
			IsPacketCommitted(mock.Anything, testRoute.SourceClientID, uint64(42)).
			Return(true, nil).
			Once()

		// check send finality: the send event's height is past what the destination prover can prove
		env.dstProver.EXPECT().LatestProvableHeight(mock.Anything).Return(uint64(50), time.Now(), nil).Once()
		env.srcClient.EXPECT().TxHeight(mock.Anything, mock.Anything).Return(uint64(100), nil).Once()

		out := runPipeline(t, deps, fastOpts(), tr)

		require.ErrorIs(t, out.ProcessingError, processors.ErrSendNotFinalized)

		// the packet stays dispatchable for the next run
		dispatchable, err := env.store.ListDispatchablePackets(context.Background())
		require.NoError(t, err)
		assert.Len(t, dispatchable, 1)
	})
}

type staticTxSubmitters map[string]txsubmitter.TxSubmitter

func (s staticTxSubmitters) Get(chainID, _ string) (txsubmitter.TxSubmitter, bool) {
	txSubmitter, ok := s[chainID]
	return txSubmitter, ok
}

func TestPipelinePushRespectsContextCancellation(t *testing.T) {
	// a zero-capacity, unread input channel simulates a full buffer: any send
	// blocks until something reads it or the context cancels.
	p := &Pipeline{input: make(chan *processors.Transfer)}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan bool, 1)
	go func() {
		done <- p.Push(ctx, &processors.Transfer{})
	}()

	select {
	case <-done:
		require.FailNow(t, "Push returned before the input was read or the context was canceled")
	case <-time.After(50 * time.Millisecond):
	}

	cancel()

	select {
	case pushed := <-done:
		assert.False(t, pushed, "Push must report failure when canceled before the send lands")
	case <-time.After(time.Second):
		require.FailNow(t, "Push did not return after its context was canceled; it is still blocked on the full buffer")
	}
}

func TestPipelineExcludesPendingPacketFromNextBatch(t *testing.T) {
	env, deps := newPipelineEnv(t)
	pending := env.createPacket(t, time.Now().Add(time.Hour))
	fresh := *pending
	fresh.PacketSequenceNumber = 43
	require.NoError(t, env.store.UpsertPacket(t.Context(), store.UpsertPacket{
		Status:                    store.RelayStatusPending,
		SourceChainID:             fresh.SourceChainID,
		DestinationChainID:        fresh.DestinationChainID,
		SourceTxHash:              fresh.SourceTxHash,
		SourceTxTime:              fresh.SourceTxTime,
		PacketSequenceNumber:      43,
		PacketSourceClientID:      fresh.PacketSourceClientID,
		PacketDestinationClientID: fresh.PacketDestinationClientID,
		PacketTimeoutTimestamp:    fresh.PacketTimeoutTimestamp,
	}))
	env.dstClient.EXPECT().
		IsPacketReceived(mock.Anything, testRoute.DestinationClientID, mock.Anything).
		Return(false, nil).
		Twice()
	env.srcClient.EXPECT().
		IsPacketCommitted(mock.Anything, testRoute.SourceClientID, mock.Anything).
		Return(true, nil).
		Twice()
	env.dstProver.EXPECT().LatestProvableHeight(mock.Anything).Return(uint64(100), time.Now(), nil)
	env.srcClient.EXPECT().TxHeight(mock.Anything, mock.Anything).Return(uint64(100), nil).Twice()
	env.dstTxSubmitter.EXPECT().
		HasPending(mock.Anything, v2.OperationMetadata{SourceChainID: "1", SourceClientID: testRoute.SourceClientID, Sequence: 42, Type: v2.OperationTypeRecv}).
		Return(true, nil).
		Once()
	env.dstTxSubmitter.EXPECT().
		HasPending(mock.Anything, v2.OperationMetadata{SourceChainID: "1", SourceClientID: testRoute.SourceClientID, Sequence: 43, Type: v2.OperationTypeRecv}).
		Return(false, nil).
		Once()
	mockRelay(env.srcClient, env.dstProver, env.dstTxBuilder, []v2.PacketEvent{sendPacketEvent(43)}, "0xrouter")
	env.dstClient.EXPECT().WaitForChain(mock.Anything).Return(nil).Once()
	env.dstTxSubmitter.EXPECT().Submit(mock.Anything, mock.MatchedBy(func(intent v2.TxIntent) bool {
		return len(intent.Operations) == 1 && intent.Operations[0].Sequence == 43
	})).Return(nil).Once()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	opts := fastOpts()
	opts.RecvBatchSize = 2
	pipeline, err := NewPipeline(ctx, slog.Default(), deps, testRoute, opts)
	require.NoError(t, err)
	require.True(t, pipeline.Push(ctx, pending))
	require.True(t, pipeline.Push(ctx, &fresh))
	outputs := make(chan *processors.Transfer, 2)
	go func() {
		for range 2 {
			tr, _ := pipeline.Poll()
			outputs <- tr
		}
	}()
	for range 2 {
		select {
		case tr := <-outputs:
			require.NotNil(t, tr)
			require.ErrorIs(t, tr.ProcessingError, processors.ErrSubmissionPending)
			require.Nil(t, tr.RecvTxHash)
		case <-time.After(5 * time.Second):
			t.Fatal("packets did not leave pipeline")
		}
	}
	pipeline.Close()
}
