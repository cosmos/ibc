// SPDX-License-Identifier: Apache-2.0

package pipeline

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/relay/processors"
	"github.com/cosmos/ibc/link/internal/relay/proofgen"
	"github.com/cosmos/ibc/link/internal/relay/txbuilder"
	"github.com/cosmos/ibc/link/internal/store"
	"github.com/cosmos/ibc/link/internal/tests/mocks"
	"github.com/cosmos/ibc/link/internal/txsubmitter"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
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
	// dstProofGen/dstTxBuilder are resolved for the destination client/chain,
	// used by recv delivery; srcProofGen/srcTxBuilder are resolved for the
	// source client/chain, used by ack and timeout delivery.
	dstProofGen    *mocks.MockProofGenerator
	srcProofGen    *mocks.MockProofGenerator
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

type staticProofGenerators map[string]proofgen.ProofGenerator

func (s staticProofGenerators) Get(chainID, clientID string) (proofgen.ProofGenerator, bool) {
	gen, ok := s[proofgen.Key(chainID, clientID)]
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
		dstProofGen:    mocks.NewMockProofGenerator(t),
		srcProofGen:    mocks.NewMockProofGenerator(t),
		dstTxBuilder:   mocks.NewMockTxBuilder(t),
		srcTxBuilder:   mocks.NewMockTxBuilder(t),
		srcTxSubmitter: mocks.NewMockTxSubmitter(t),
		dstTxSubmitter: mocks.NewMockTxSubmitter(t),
	}

	deps := Deps{
		Storage: db,
		Chains:  staticChains{testRoute.SourceChainID: env.srcClient, testRoute.DestinationChainID: env.dstClient},
		ProofGenerators: staticProofGenerators{
			proofgen.Key(testRoute.DestinationChainID, testRoute.DestinationClientID): env.dstProofGen,
			proofgen.Key(testRoute.SourceChainID, testRoute.SourceClientID):           env.srcProofGen,
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
	proofGen *mocks.MockProofGenerator,
	txBuilder *mocks.MockTxBuilder,
	events []v2.PacketEvent,
	to string,
) {
	const height = uint64(100)

	client.EXPECT().TxPacketEvents(mock.Anything, mock.Anything).Return(events, nil).Once()
	proofGen.EXPECT().LatestProvableHeight(mock.Anything).Return(height, time.Now(), nil).Once()
	proofGen.EXPECT().StateProof(mock.Anything, height).Return([]byte{0x01}, nil).Once()
	proofGen.EXPECT().PacketProofs(mock.Anything, height, mock.Anything, mock.Anything).
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

func TestPipelineLifecycle(t *testing.T) {
	ctx := context.Background()

	t.Run("recvToWriteAckSuccessComplete", func(t *testing.T) {
		env, deps := newPipelineEnv(t)
		tr := env.createPacket(t, time.Now().Add(time.Hour))

		// packet not yet received, commitment present, send finalized
		env.dstClient.EXPECT().
			IsPacketReceived(mock.Anything, testRoute.DestinationClientID, uint64(42)).
			Return(false, nil).
			Once()
		env.srcClient.EXPECT().
			IsPacketCommitted(mock.Anything, testRoute.SourceClientID, uint64(42)).
			Return(true, nil).
			Times(2)
		env.srcClient.EXPECT().IsTxFinalized(mock.Anything, tr.SourceTxHash, (*uint64)(nil)).Return(true, nil).Once()

		// recv delivery
		mockRelay(env.srcClient, env.dstProofGen, env.dstTxBuilder, []v2.PacketEvent{sendPacketEvent(42)}, "0xrouter")
		env.dstClient.EXPECT().WaitForChain(mock.Anything).Return(nil).Once()
		env.dstTxSubmitter.EXPECT().Submit(mock.Anything, mock.Anything).Return(&v2.Submission{
			TxHash:         recvTxHash,
			SubmittedAt:    time.Now().UTC(),
			RelayerAddress: "0xrelayer",
		}, nil).Once()
		env.dstTxSubmitter.EXPECT().ShouldRetry(mock.Anything, recvTxHash, mock.Anything).Return(false, nil).Once()

		// success write ack: relayed back to the source chain like any other ack
		env.dstClient.EXPECT().
			PacketWriteAckStatus(mock.Anything, recvTxHash, uint64(42), testRoute.SourceClientID, testRoute.DestinationClientID).
			Return(chainsWriteAckSuccess(), nil).
			Once()
		env.dstClient.EXPECT().IsTxFinalized(mock.Anything, recvTxHash, (*uint64)(nil)).Return(true, nil).Once()

		// ack delivery on the source chain
		mockRelay(env.dstClient, env.srcProofGen, env.srcTxBuilder, []v2.PacketEvent{writeAckEvent(42)}, "0xrouter")
		env.srcClient.EXPECT().WaitForChain(mock.Anything).Return(nil).Once()
		env.srcTxSubmitter.EXPECT().Submit(mock.Anything, mock.Anything).Return(&v2.Submission{
			TxHash: ackTxHash, SubmittedAt: time.Now().UTC(), RelayerAddress: "0xrelayer",
		}, nil).Once()
		env.srcTxSubmitter.EXPECT().ShouldRetry(mock.Anything, ackTxHash, mock.Anything).Return(false, nil).Once()

		out := runPipeline(t, deps, fastOpts(), tr)

		require.Empty(t, out.Error())
		assert.Equal(t, store.RelayStatusCompleteWithAck, out.Status)

		stored := env.storedPacket(t, out)
		assert.Equal(t, store.RelayStatusCompleteWithAck, stored.Status)
		require.NotNil(t, stored.RecvTxHash)
		assert.Equal(t, recvTxHash, *stored.RecvTxHash)
		require.NotNil(t, stored.WriteAckStatus)
		assert.Equal(t, store.WriteAckStatusSuccess, *stored.WriteAckStatus)
		_ = ctx
	})

	t.Run("errorAckRelayedToComplete", func(t *testing.T) {
		env, deps := newPipelineEnv(t)
		tr := env.createPacket(t, time.Now().Add(time.Hour))

		env.dstClient.EXPECT().
			IsPacketReceived(mock.Anything, testRoute.DestinationClientID, uint64(42)).
			Return(false, nil).
			Once()
		env.srcClient.EXPECT().
			IsPacketCommitted(mock.Anything, testRoute.SourceClientID, uint64(42)).
			Return(true, nil).
			Times(2)
		env.srcClient.EXPECT().IsTxFinalized(mock.Anything, tr.SourceTxHash, (*uint64)(nil)).Return(true, nil).Once()

		// recv delivery
		mockRelay(env.srcClient, env.dstProofGen, env.dstTxBuilder, []v2.PacketEvent{sendPacketEvent(42)}, "0xrouter")
		env.dstClient.EXPECT().WaitForChain(mock.Anything).Return(nil).Once()
		env.dstTxSubmitter.EXPECT().Submit(mock.Anything, mock.Anything).Return(&v2.Submission{
			TxHash: recvTxHash, SubmittedAt: time.Now().UTC(), RelayerAddress: "0xrelayer",
		}, nil).Once()
		env.dstTxSubmitter.EXPECT().ShouldRetry(mock.Anything, recvTxHash, mock.Anything).Return(false, nil).Once()

		// error write ack: relayed back to the source chain
		env.dstClient.EXPECT().
			PacketWriteAckStatus(mock.Anything, recvTxHash, uint64(42), testRoute.SourceClientID, testRoute.DestinationClientID).
			Return(chainsWriteAckError(), nil).
			Once()
		env.dstClient.EXPECT().IsTxFinalized(mock.Anything, recvTxHash, (*uint64)(nil)).Return(true, nil).Once()

		// ack delivery on the source chain
		mockRelay(env.dstClient, env.srcProofGen, env.srcTxBuilder, []v2.PacketEvent{writeAckEvent(42)}, "0xrouter")
		env.srcClient.EXPECT().WaitForChain(mock.Anything).Return(nil).Once()
		env.srcTxSubmitter.EXPECT().Submit(mock.Anything, mock.Anything).Return(&v2.Submission{
			TxHash: ackTxHash, SubmittedAt: time.Now().UTC(), RelayerAddress: "0xrelayer",
		}, nil).Once()
		env.srcTxSubmitter.EXPECT().ShouldRetry(mock.Anything, ackTxHash, mock.Anything).Return(false, nil).Once()

		out := runPipeline(t, deps, fastOpts(), tr)

		require.Empty(t, out.Error())
		assert.Equal(t, store.RelayStatusCompleteWithWriteAckError, out.Status)

		stored := env.storedPacket(t, out)
		assert.Equal(t, store.RelayStatusCompleteWithWriteAckError, stored.Status)
		require.NotNil(t, stored.AckTxHash)
		assert.Equal(t, ackTxHash, *stored.AckTxHash)
	})

	t.Run("timedOutPacketCompletesWithTimeout", func(t *testing.T) {
		env, deps := newPipelineEnv(t)
		tr := env.createPacket(t, time.Now().Add(-time.Hour))

		env.dstClient.EXPECT().
			IsPacketReceived(mock.Anything, testRoute.DestinationClientID, uint64(42)).
			Return(false, nil).
			Once()
		env.srcClient.EXPECT().
			IsPacketCommitted(mock.Anything, testRoute.SourceClientID, uint64(42)).
			Return(true, nil).
			Once()
		env.srcClient.EXPECT().IsTxFinalized(mock.Anything, tr.SourceTxHash, (*uint64)(nil)).Return(true, nil).Once()
		env.dstClient.EXPECT().
			IsTimestampFinalized(mock.Anything, mock.Anything, (*uint64)(nil)).
			Return(true, nil).
			Once()

		// timeout delivery on the source chain
		timeoutEvent := sendPacketEvent(42)
		timeoutEvent.Height = 150 // source height is unrelated to the destination proof height
		mockRelay(env.srcClient, env.srcProofGen, env.srcTxBuilder, []v2.PacketEvent{timeoutEvent}, "0xrouter")
		env.srcClient.EXPECT().WaitForChain(mock.Anything).Return(nil).Once()
		env.srcTxSubmitter.EXPECT().Submit(mock.Anything, mock.Anything).Return(&v2.Submission{
			TxHash: timeoutTxHash, SubmittedAt: time.Now().UTC(), RelayerAddress: "0xrelayer",
		}, nil).Once()
		env.srcTxSubmitter.EXPECT().ShouldRetry(mock.Anything, timeoutTxHash, mock.Anything).Return(false, nil).Once()

		out := runPipeline(t, deps, fastOpts(), tr)

		require.Empty(t, out.Error())
		assert.Equal(t, store.RelayStatusCompleteWithTimeout, out.Status)

		stored := env.storedPacket(t, out)
		assert.Equal(t, store.RelayStatusCompleteWithTimeout, stored.Status)
		require.NotNil(t, stored.TimeoutTxHash)
		assert.Equal(t, timeoutTxHash, *stored.TimeoutTxHash)
		assert.Nil(t, stored.RecvTxHash)
	})

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
		env.srcClient.EXPECT().IsTxFinalized(mock.Anything, tr.SourceTxHash, (*uint64)(nil)).Return(false, nil).Once()

		out := runPipeline(t, deps, fastOpts(), tr)

		require.ErrorIs(t, out.ProcessingError, processors.ErrSendNotFinalized)

		// the packet stays dispatchable for the next run
		dispatchable, err := env.store.ListDispatchablePackets(context.Background())
		require.NoError(t, err)
		assert.Len(t, dispatchable, 1)
	})
}

func chainsWriteAckSuccess() v2.WriteAckStatus { return v2.WriteAckStatusSuccess }
func chainsWriteAckError() v2.WriteAckStatus   { return v2.WriteAckStatusError }

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
