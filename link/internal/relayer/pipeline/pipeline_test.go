package pipeline

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/cosmos/ibc/link/internal/relayer/processors"
	"github.com/cosmos/ibc/link/internal/relayer/transfer"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/store"
	"github.com/cosmos/ibc/link/internal/txmgr"
	proto "github.com/cosmos/ibc/link/internal/types/proofapi"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

const (
	recvTxHash    = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	ackTxHash     = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	timeoutTxHash = "0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
)

var testRoute = transfer.Route{
	SourceChainID:       "1",
	SourceClientID:      "base-0",
	DestinationChainID:  "8453",
	DestinationClientID: "ethereum-0",
}

type pipelineEnv struct {
	store        *store.SqliteDB
	srcClient    *chains.MockClient
	dstClient    *chains.MockClient
	proofAPI     *proto.MockProofApiServiceClient
	srcSubmitter *txmgr.MockTxManager
	dstSubmitter *txmgr.MockTxManager
}

type staticChains map[string]chains.Client

func (s staticChains) Get(chainID string) (chains.Client, bool) {
	client, ok := s[chainID]
	return client, ok
}

func newPipelineEnv(t *testing.T) (*pipelineEnv, Deps) {
	t.Helper()

	db, err := store.NewSqliteInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.MigrateUp()
	require.NoError(t, err)

	env := &pipelineEnv{
		store:        db,
		srcClient:    chains.NewMockClient(t),
		dstClient:    chains.NewMockClient(t),
		proofAPI:     proto.NewMockProofApiServiceClient(t),
		srcSubmitter: txmgr.NewMockTxManager(t),
		dstSubmitter: txmgr.NewMockTxManager(t),
	}

	deps := Deps{
		Storage:  db,
		Chains:   staticChains{testRoute.SourceChainID: env.srcClient, testRoute.DestinationChainID: env.dstClient},
		ProofAPI: env.proofAPI,
		TxManagers: staticTxManagers{
			testRoute.SourceChainID:      env.srcSubmitter,
			testRoute.DestinationChainID: env.dstSubmitter,
		},
	}

	return env, deps
}

func (env *pipelineEnv) createPacket(t *testing.T, timeout time.Time) *transfer.Transfer {
	t.Helper()

	ctx := context.Background()
	input := store.CreatePacket{
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
	require.NoError(t, env.store.CreatePacket(ctx, input))

	packets, err := env.store.ListPacketsBySourceTx(ctx, input.SourceChainID, input.SourceTxHash)
	require.NoError(t, err)
	require.Len(t, packets, 1)

	return transfer.NewTransfer(packets[0], slog.Default())
}

func (env *pipelineEnv) storedPacket(t *testing.T, tr *transfer.Transfer) store.Packet {
	t.Helper()

	packets, err := env.store.ListPacketsBySourceTx(context.Background(), tr.SourceChainID, tr.SourceTxHash)
	require.NoError(t, err)
	require.Len(t, packets, 1)

	return packets[0]
}

func fastOpts() Options {
	return Options{
		RecvBatchSize:       1,
		RecvBatchTimeout:    50 * time.Millisecond,
		AckBatchSize:        1,
		AckBatchTimeout:     50 * time.Millisecond,
		TimeoutBatchSize:    1,
		TimeoutBatchTimeout: 50 * time.Millisecond,
	}
}

func relayResponse(to string) *connect.Response[proto.RelayByTxResponse] {
	return connect.NewResponse(&proto.RelayByTxResponse{Tx: []byte{0xca, 0x11}, Address: to})
}

func runPipeline(t *testing.T, deps Deps, opts Options, tr *transfer.Transfer) *transfer.Transfer {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p, err := NewPipeline(ctx, slog.Default(), deps, testRoute, opts)
	require.NoError(t, err)
	require.True(t, p.Push(ctx, tr))

	done := make(chan *transfer.Transfer, 1)
	go func() {
		out, err := p.Poll()
		require.NoError(t, err)
		done <- out
	}()

	select {
	case out := <-done:
		p.Close()

		return out
	case <-time.After(15 * time.Second):
		t.Fatal("pipeline did not emit the tr in time")

		return nil
	}
}

func TestPipelineLifecycle(t *testing.T) {
	ctx := context.Background()

	t.Run("recvToWriteAckSuccessComplete", func(t *testing.T) {
		env, deps := newPipelineEnv(t)
		tr := env.createPacket(t, time.Now().Add(time.Hour))

		// packet not yet received, commitment present, send finalized
		env.dstClient.EXPECT().IsPacketReceived(mock.Anything, testRoute.DestinationClientID, uint64(42)).Return(false, nil).Once()
		env.srcClient.EXPECT().IsPacketCommitted(mock.Anything, testRoute.SourceClientID, uint64(42)).Return(true, nil).Times(2)
		env.srcClient.EXPECT().IsTxFinalized(mock.Anything, tr.SourceTxHash, (*uint64)(nil)).Return(true, nil).Once()

		// recv then ack delivery via proof api
		env.proofAPI.EXPECT().RelayByTx(mock.Anything, mock.Anything).Return(relayResponse("0xrouter"), nil).Times(2)
		env.dstClient.EXPECT().WaitForChain(mock.Anything).Return(nil).Once()
		env.dstSubmitter.EXPECT().Submit(mock.Anything, mock.Anything).Return(&v2.Submission{
			TxHash:         recvTxHash,
			SubmittedAt:    time.Now().UTC(),
			RelayerAddress: "0xrelayer",
		}, nil).Once()
		env.dstSubmitter.EXPECT().ShouldRetry(mock.Anything, recvTxHash, processors.RetryRecvExpiry, mock.Anything).Return(false, nil).Once()

		// success write ack: relayed back to the source chain like any other ack
		env.dstClient.EXPECT().PacketWriteAckStatus(mock.Anything, recvTxHash, uint64(42), testRoute.SourceClientID, testRoute.DestinationClientID).
			Return(chainsWriteAckSuccess(), nil).Once()
		env.dstClient.EXPECT().IsTxFinalized(mock.Anything, recvTxHash, (*uint64)(nil)).Return(true, nil).Once()

		// ack delivery on the source chain
		env.srcClient.EXPECT().WaitForChain(mock.Anything).Return(nil).Once()
		env.srcSubmitter.EXPECT().Submit(mock.Anything, mock.Anything).Return(&v2.Submission{
			TxHash: ackTxHash, SubmittedAt: time.Now().UTC(), RelayerAddress: "0xrelayer",
		}, nil).Once()
		env.srcSubmitter.EXPECT().ShouldRetry(mock.Anything, ackTxHash, processors.RetryAckExpiry, mock.Anything).Return(false, nil).Once()

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

		env.dstClient.EXPECT().IsPacketReceived(mock.Anything, testRoute.DestinationClientID, uint64(42)).Return(false, nil).Once()
		env.srcClient.EXPECT().IsPacketCommitted(mock.Anything, testRoute.SourceClientID, uint64(42)).Return(true, nil).Times(2)
		env.srcClient.EXPECT().IsTxFinalized(mock.Anything, tr.SourceTxHash, (*uint64)(nil)).Return(true, nil).Once()

		// recv delivery
		env.proofAPI.EXPECT().RelayByTx(mock.Anything, mock.Anything).Return(relayResponse("0xrouter"), nil).Times(2)
		env.dstClient.EXPECT().WaitForChain(mock.Anything).Return(nil).Once()
		env.dstSubmitter.EXPECT().Submit(mock.Anything, mock.Anything).Return(&v2.Submission{
			TxHash: recvTxHash, SubmittedAt: time.Now().UTC(), RelayerAddress: "0xrelayer",
		}, nil).Once()
		env.dstSubmitter.EXPECT().ShouldRetry(mock.Anything, recvTxHash, processors.RetryRecvExpiry, mock.Anything).Return(false, nil).Once()

		// error write ack: relayed back to the source chain
		env.dstClient.EXPECT().PacketWriteAckStatus(mock.Anything, recvTxHash, uint64(42), testRoute.SourceClientID, testRoute.DestinationClientID).
			Return(chainsWriteAckError(), nil).Once()
		env.dstClient.EXPECT().IsTxFinalized(mock.Anything, recvTxHash, (*uint64)(nil)).Return(true, nil).Once()

		// ack delivery on the source chain
		env.srcClient.EXPECT().WaitForChain(mock.Anything).Return(nil).Once()
		env.srcSubmitter.EXPECT().Submit(mock.Anything, mock.Anything).Return(&v2.Submission{
			TxHash: ackTxHash, SubmittedAt: time.Now().UTC(), RelayerAddress: "0xrelayer",
		}, nil).Once()
		env.srcSubmitter.EXPECT().ShouldRetry(mock.Anything, ackTxHash, processors.RetryAckExpiry, mock.Anything).Return(false, nil).Once()

		out := runPipeline(t, deps, fastOpts(), tr)

		require.Empty(t, out.Error())
		assert.Equal(t, store.RelayStatusCompleteWithAck, out.Status)

		stored := env.storedPacket(t, out)
		assert.Equal(t, store.RelayStatusCompleteWithAck, stored.Status)
		require.NotNil(t, stored.AckTxHash)
		assert.Equal(t, ackTxHash, *stored.AckTxHash)
	})

	t.Run("timedOutPacketCompletesWithTimeout", func(t *testing.T) {
		env, deps := newPipelineEnv(t)
		tr := env.createPacket(t, time.Now().Add(-time.Hour))

		env.dstClient.EXPECT().IsPacketReceived(mock.Anything, testRoute.DestinationClientID, uint64(42)).Return(false, nil).Once()
		env.srcClient.EXPECT().IsPacketCommitted(mock.Anything, testRoute.SourceClientID, uint64(42)).Return(true, nil).Once()
		env.srcClient.EXPECT().IsTxFinalized(mock.Anything, tr.SourceTxHash, (*uint64)(nil)).Return(true, nil).Once()
		env.dstClient.EXPECT().IsTimestampFinalized(mock.Anything, mock.Anything, (*uint64)(nil)).Return(true, nil).Once()

		// timeout delivery on the source chain
		env.proofAPI.EXPECT().RelayByTx(mock.Anything, mock.Anything).Return(relayResponse("0xrouter"), nil).Once()
		env.srcClient.EXPECT().WaitForChain(mock.Anything).Return(nil).Once()
		env.srcSubmitter.EXPECT().Submit(mock.Anything, mock.Anything).Return(&v2.Submission{
			TxHash: timeoutTxHash, SubmittedAt: time.Now().UTC(), RelayerAddress: "0xrelayer",
		}, nil).Once()
		env.srcSubmitter.EXPECT().ShouldRetry(mock.Anything, timeoutTxHash, processors.RetryTimeoutExpiry, mock.Anything).Return(false, nil).Once()

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

		env.dstClient.EXPECT().IsPacketReceived(mock.Anything, testRoute.DestinationClientID, uint64(42)).Return(false, nil).Once()
		env.srcClient.EXPECT().IsPacketCommitted(mock.Anything, testRoute.SourceClientID, uint64(42)).Return(true, nil).Once()
		env.srcClient.EXPECT().IsTxFinalized(mock.Anything, tr.SourceTxHash, (*uint64)(nil)).Return(false, nil).Once()

		out := runPipeline(t, deps, fastOpts(), tr)

		assert.ErrorIs(t, out.ProcessingError, transfer.ErrSendNotFinalized)

		// the packet stays unfinished for the next run
		unfinished, err := env.store.ListUnfinishedPackets(context.Background())
		require.NoError(t, err)
		assert.Len(t, unfinished, 1)
	})
}

func chainsWriteAckSuccess() v2.WriteAckStatus { return v2.WriteAckStatusSuccess }
func chainsWriteAckError() v2.WriteAckStatus   { return v2.WriteAckStatusError }

// routedConfig a config whose clients and routes cover testRoute.
func routedConfig() config.Config {
	return config.Config{
		Relayer: config.RelayerConfig{
			Clients: []config.ClientConfig{
				{
					Alias:                "test-route",
					ClientID:             testRoute.SourceClientID,
					ChainID:              testRoute.SourceChainID,
					CounterpartyChainID:  testRoute.DestinationChainID,
					CounterpartyClientID: testRoute.DestinationClientID,
					Type:                 config.ClientTypeAttestation,
				},
			},
			Routes: []config.RouteConfig{{SourceClient: "test-route"}},
		},
	}
}

type staticTxManagers map[string]txmgr.TxManager

func (s staticTxManagers) Get(chainID string) (txmgr.TxManager, bool) {
	submitter, ok := s[chainID]
	return submitter, ok
}
