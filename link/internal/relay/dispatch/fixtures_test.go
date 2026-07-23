package dispatch

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/relay/pipeline"
	"github.com/cosmos/ibc/link/internal/relay/processors"
	"github.com/cosmos/ibc/link/internal/store"
	"github.com/cosmos/ibc/link/internal/tests/mocks"
	"github.com/cosmos/ibc/link/internal/txmgr"
	proto "github.com/cosmos/ibc/link/internal/types/proofapi"
)

func testTransfer(t *testing.T) *processors.Transfer {
	t.Helper()

	return processors.NewTransfer(store.Packet{
		Status:                    store.RelayStatusPending,
		SourceChainID:             testRoute.SourceChainID,
		DestinationChainID:        testRoute.DestinationChainID,
		SourceTxHash:              "0xsend",
		PacketSequenceNumber:      42,
		PacketSourceClientID:      testRoute.SourceClientID,
		PacketDestinationClientID: testRoute.DestinationClientID,
		PacketTimeoutTimestamp:    time.Now().Add(time.Hour),
	}, slog.Default())
}

type staticChains map[string]chains.Client

func (s staticChains) Get(chainID string) (chains.Client, bool) {
	client, ok := s[chainID]
	return client, ok
}

type pipelineEnv struct {
	store *store.SqliteDB
}

func newPipelineEnv(t *testing.T) (*pipelineEnv, pipeline.Deps) {
	t.Helper()

	db, err := store.NewSqliteInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.MigrateUp()
	require.NoError(t, err)

	deps := pipeline.Deps{
		Storage: db,
		Chains: staticChains{
			testRoute.SourceChainID:      mocks.NewMockClient(t),
			testRoute.DestinationChainID: mocks.NewMockClient(t),
		},
		ProofAPI: proto.NewMockProofApiServiceClient(t),
		TxManagers: staticTxManagers{
			testRoute.SourceChainID:      txmgr.NewMockTxManager(t),
			testRoute.DestinationChainID: txmgr.NewMockTxManager(t),
		},
	}

	return &pipelineEnv{store: db}, deps
}

func (env *pipelineEnv) createPacket(t *testing.T, timeout time.Time) *processors.Transfer {
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

	return processors.NewTransfer(packets[0], slog.Default())
}

type staticTxManagers map[string]txmgr.TxManager

func (s staticTxManagers) Get(chainID, _ string) (txmgr.TxManager, bool) {
	txManager, ok := s[chainID]
	return txManager, ok
}
