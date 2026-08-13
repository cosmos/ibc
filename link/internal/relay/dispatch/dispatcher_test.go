// SPDX-License-Identifier: Apache-2.0

package dispatch

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/relay/pipeline"
	"github.com/cosmos/ibc/link/internal/relay/processors"
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
			Connections: []config.ConnectionConfig{
				{
					Alias: "test-route",
					ClientA: config.ClientEnd{
						ChainID:  testRoute.SourceChainID,
						ClientID: testRoute.SourceClientID,
						Signer:   "test-signer",
						Type:     config.ClientTypeAttestation,
					},
					ClientB: config.ClientEnd{
						ChainID:  testRoute.DestinationChainID,
						ClientID: testRoute.DestinationClientID,
						Signer:   "test-signer",
						Type:     config.ClientTypeAttestation,
					},
				},
			},
		},
	}
}

// fakePipeline records pushes and lets tests control Push acceptance.
type fakePipeline struct {
	mu     sync.Mutex
	pushed []*processors.Transfer
	accept bool
	closed bool
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

func (p *fakePipeline) Close() {
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()

	close(p.out)
}

func (p *fakePipeline) pushCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return len(p.pushed)
}

type fakePipelines struct {
	pipeline pipeline.TransferPipeline
	err      error
	closed   bool
}

func (r *fakePipelines) Pipeline(context.Context, *processors.Transfer) (pipeline.TransferPipeline, error) {
	if r.err != nil {
		return nil, r.err
	}

	return r.pipeline, nil
}

func (r *fakePipelines) Close() { r.closed = true }

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

	require.NoError(t, db.UpsertPacket(context.Background(), store.UpsertPacket{
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

	t.Run("submitsDispatchablePackets", func(t *testing.T) {
		db := dispatcherStore(t)
		createStoredPacket(t, db, 1)
		createStoredPacket(t, db, 2)

		pipe := newFakePipeline(true)
		dispatcher := NewRelayDispatcher(db, &fakePipelines{pipeline: pipe}, DefaultPollInterval, slog.Default())

		require.NoError(t, dispatcher.SubmitWaitingDispatchablePackets(ctx))

		assert.Equal(t, 2, pipe.pushCount())
	})

	t.Run("alreadyInPipelineIsSwallowed", func(t *testing.T) {
		db := dispatcherStore(t)
		createStoredPacket(t, db, 1)

		pipe := newFakePipeline(false) // rejects: already in pipeline
		dispatcher := NewRelayDispatcher(db, &fakePipelines{pipeline: pipe}, DefaultPollInterval, slog.Default())

		require.NoError(t, dispatcher.SubmitWaitingDispatchablePackets(ctx))

		// the packet is not marked failed
		dispatchable, err := db.ListDispatchablePackets(ctx)
		require.NoError(t, err)
		assert.Len(t, dispatchable, 1)
		assert.Equal(t, store.RelayStatusPending, dispatchable[0].Status)
	})

	t.Run("submitErrorMarksPacketFailed", func(t *testing.T) {
		db := dispatcherStore(t)
		createStoredPacket(t, db, 1)

		dispatcher := NewRelayDispatcher(
			db,
			&fakePipelines{err: errors.New("no route")},
			DefaultPollInterval,
			slog.Default(),
		)

		require.NoError(t, dispatcher.SubmitWaitingDispatchablePackets(ctx))

		dispatchable, err := db.ListDispatchablePackets(ctx)
		require.NoError(t, err)
		assert.Empty(t, dispatchable)

		packets, err := db.ListPacketsBySourceTx(ctx, testRoute.SourceChainID, recvTxHash)
		require.NoError(t, err)
		require.Len(t, packets, 1)
		assert.Equal(t, store.RelayStatusFailed, packets[0].Status)
	})

	t.Run("startStopManagesItsOwnContext", func(t *testing.T) {
		db := dispatcherStore(t)
		pipelines := &fakePipelines{pipeline: newFakePipeline(true)}
		dispatcher := NewRelayDispatcher(db, pipelines, time.Millisecond, slog.Default())

		require.NoError(t, dispatcher.Start())
		require.NoError(t, dispatcher.Stop(), "Stop must block until the loop has exited and the pipelines are closed")
		assert.True(t, pipelines.closed)
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

		// once the transfer exits the pipeline it can be pushed again
		inner.out <- tr
		require.Eventually(t, func() bool {
			return deduper.Push(ctx, tr)
		}, 5*time.Second, 10*time.Millisecond)

		deduper.Close()
	})
}

func TestPipelineSet(t *testing.T) {
	ctx := context.Background()

	t.Run("memoizesPipelinesPerRoute", func(t *testing.T) {
		env, deps := newPipelineEnv(t)

		pipelineCtx, cancel := context.WithCancel(ctx)
		pipelines := NewPipelineSet(slog.Default(), routedConfig(), deps)
		// pipeline outputs only close on cancellation; cancel before Close
		t.Cleanup(func() { cancel(); pipelines.Close() })

		tr := env.createPacket(t, time.Now().Add(time.Hour))

		first, err := pipelines.Pipeline(pipelineCtx, tr)
		require.NoError(t, err)

		second, err := pipelines.Pipeline(pipelineCtx, tr)
		require.NoError(t, err)

		assert.Same(t, first, second)
	})

	t.Run("rejectsUnroutedTransfers", func(t *testing.T) {
		env, deps := newPipelineEnv(t)

		pipelines := NewPipelineSet(slog.Default(), routedConfig(), deps)

		tr := env.createPacket(t, time.Now().Add(time.Hour))
		tr.PacketSourceClientID = "unknown-0"

		_, err := pipelines.Pipeline(ctx, tr)

		require.ErrorContains(t, err, "no route configured")
	})
}
