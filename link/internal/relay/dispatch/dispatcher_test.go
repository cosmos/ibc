package dispatch

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/relay/pipeline"
	"github.com/cosmos/ibc/link/internal/relay/processors"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/store"
)

const recvTxHash = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

var testRoute = processors.Route{
	SourceChainID:       "1",
	SourceClientID:      "base-0",
	DestinationChainID:  "8453",
	DestinationClientID: "ethereum-0",
}

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
			Routes: []config.RouteConfig{{SourceClient: "test-route", SourceSignerAlias: "test-signer", DestSignerAlias: "test-signer"}},
		},
	}
}

// fakePipeline records pushes and lets tests control Push acceptance.
type fakePipeline struct {
	mu     sync.Mutex
	pushed []*processors.Transfer
	accept bool
	out    chan *processors.Transfer
}

func newFakePipeline(accept bool) *fakePipeline {
	return &fakePipeline{accept: accept, out: make(chan *processors.Transfer, 100)}
}

func (p *fakePipeline) Push(_ context.Context, t *processors.Transfer) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.accept {
		return false
	}

	p.pushed = append(p.pushed, t)

	return true
}

func (p *fakePipeline) Poll() (*processors.Transfer, error) {
	t, ok := <-p.out
	if !ok {
		return nil, errors.New("closed")
	}

	return t, nil
}

func (p *fakePipeline) Close() { close(p.out) }

func (p *fakePipeline) pushCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return len(p.pushed)
}

type fakeManager struct {
	pipeline pipeline.TransferPipeline
	err      error
	closed   bool
}

func (m *fakeManager) Pipeline(context.Context, *processors.Transfer) (pipeline.TransferPipeline, error) {
	if m.err != nil {
		return nil, m.err
	}

	return m.pipeline, nil
}

func (m *fakeManager) Close() { m.closed = true }

func dispatcherStore(t *testing.T) *store.SqliteDB {
	t.Helper()

	db, err := store.NewSqliteInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.MigrateUp()
	require.NoError(t, err)

	return db
}

func createStoredPacket(t *testing.T, db *store.SqliteDB, sequence uint64) {
	t.Helper()

	require.NoError(t, db.CreatePacket(context.Background(), store.CreatePacket{
		Status:                    store.RelayStatusPending,
		SourceChainID:             testRoute.SourceChainID,
		DestinationChainID:        testRoute.DestinationChainID,
		SourceTxHash:              recvTxHash,
		SourceTxTime:              time.Now().UTC(),
		PacketSequenceNumber:      sequence,
		PacketSourceClientID:      testRoute.SourceClientID,
		PacketDestinationClientID: testRoute.DestinationClientID,
		PacketTimeoutTimestamp:    time.Now().Add(time.Hour),
	}))
}

func TestRelayDispatcher(t *testing.T) {
	ctx := context.Background()

	t.Run("submitsUnfinishedPackets", func(t *testing.T) {
		db := dispatcherStore(t)
		createStoredPacket(t, db, 1)
		createStoredPacket(t, db, 2)

		pipe := newFakePipeline(true)
		dispatcher := NewRelayDispatcher(db, &fakeManager{pipeline: pipe}, DefaultPollInterval, slog.Default())

		require.NoError(t, dispatcher.SubmitWaitingUnfinishedPackets(ctx))

		assert.Equal(t, 2, pipe.pushCount())
	})

	t.Run("alreadyInPipelineIsSwallowed", func(t *testing.T) {
		db := dispatcherStore(t)
		createStoredPacket(t, db, 1)

		pipe := newFakePipeline(false) // rejects: already in pipeline
		dispatcher := NewRelayDispatcher(db, &fakeManager{pipeline: pipe}, DefaultPollInterval, slog.Default())

		require.NoError(t, dispatcher.SubmitWaitingUnfinishedPackets(ctx))

		// the packet is not marked failed
		unfinished, err := db.ListUnfinishedPackets(ctx)
		require.NoError(t, err)
		assert.Len(t, unfinished, 1)
		assert.Equal(t, store.RelayStatusPending, unfinished[0].Status)
	})

	t.Run("submitErrorMarksPacketFailed", func(t *testing.T) {
		db := dispatcherStore(t)
		createStoredPacket(t, db, 1)

		dispatcher := NewRelayDispatcher(db, &fakeManager{err: errors.New("no route")}, DefaultPollInterval, slog.Default())

		require.NoError(t, dispatcher.SubmitWaitingUnfinishedPackets(ctx))

		unfinished, err := db.ListUnfinishedPackets(ctx)
		require.NoError(t, err)
		assert.Empty(t, unfinished)

		packets, err := db.ListPacketsBySourceTx(ctx, testRoute.SourceChainID, recvTxHash)
		require.NoError(t, err)
		require.Len(t, packets, 1)
		assert.Equal(t, store.RelayStatusFailed, packets[0].Status)
	})

	t.Run("runStopsAndClosesManager", func(t *testing.T) {
		db := dispatcherStore(t)
		manager := &fakeManager{pipeline: newFakePipeline(true)}
		dispatcher := NewRelayDispatcher(db, manager, time.Millisecond, slog.Default())

		runCtx, cancel := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { done <- dispatcher.Run(runCtx) }()

		time.Sleep(20 * time.Millisecond)
		cancel()

		select {
		case err := <-done:
			require.ErrorIs(t, err, context.Canceled)
			assert.True(t, manager.closed)
		case <-time.After(5 * time.Second):
			t.Fatal("dispatcher did not stop")
		}
	})
}

func TestPipelineDeduper(t *testing.T) {
	ctx := context.Background()

	t.Run("rejectsDuplicatePushes", func(t *testing.T) {
		inner := newFakePipeline(true)
		deduper := NewDeduper(inner)

		tr := testTransfer(t)

		assert.True(t, deduper.Push(ctx, tr))
		assert.False(t, deduper.Push(ctx, tr))
		assert.Equal(t, 1, inner.pushCount())

		// once the tr exits the pipeline it can be pushed again
		inner.out <- tr
		require.Eventually(t, func() bool {
			return deduper.Push(ctx, tr)
		}, 5*time.Second, 10*time.Millisecond)

		deduper.Close()
	})
}

func TestManager(t *testing.T) {
	ctx := context.Background()

	t.Run("memoizesPipelinesPerRoute", func(t *testing.T) {
		env, deps := newPipelineEnv(t)

		pipelineCtx, cancel := context.WithCancel(ctx)
		manager := NewManager(slog.Default(), routedConfig(), deps)
		// pipeline outputs only close on cancellation; cancel before Close
		t.Cleanup(func() { cancel(); manager.Close() })

		tr := env.createPacket(t, time.Now().Add(time.Hour))

		first, err := manager.Pipeline(pipelineCtx, tr)
		require.NoError(t, err)

		second, err := manager.Pipeline(pipelineCtx, tr)
		require.NoError(t, err)

		assert.Same(t, first, second)
	})

	t.Run("rejectsUnroutedTransfers", func(t *testing.T) {
		env, deps := newPipelineEnv(t)

		manager := NewManager(slog.Default(), routedConfig(), deps)

		tr := env.createPacket(t, time.Now().Add(time.Hour))
		tr.PacketSourceClientID = "unknown-0"

		_, err := manager.Pipeline(ctx, tr)

		require.ErrorContains(t, err, "no route configured")
	})
}
