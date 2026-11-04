// SPDX-License-Identifier: Apache-2.0

package processors

import (
	"context"
	"encoding/hex"
	"log/slog"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	"github.com/cosmos/ibc/cli/internal/chains"
	"github.com/cosmos/ibc/cli/internal/relay/prover"
	"github.com/cosmos/ibc/cli/internal/relay/txbuilder"
	"github.com/cosmos/ibc/cli/internal/service/attestor"
	"github.com/cosmos/ibc/cli/internal/store"
	"github.com/cosmos/ibc/cli/internal/tests/mocks"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

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

// TestBatchRecvPacketSequenceAlignment guards against a regression where a
// transfer whose tx hash fails to decode still contributed its sequence
// number to the packet event filter, leaving events out of sync with the
// batch's actual tx ids.
func TestBatchRecvPacketSequenceAlignment(t *testing.T) {
	route := Route{
		SourceChainID:       "1",
		SourceClientID:      "base-0",
		DestinationChainID:  "8453",
		DestinationClientID: "ethereum-0",
	}

	valid := NewTransfer(store.Packet{
		SourceChainID:             route.SourceChainID,
		DestinationChainID:        route.DestinationChainID,
		SourceTxHash:              "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		PacketSequenceNumber:      1,
		PacketSourceClientID:      route.SourceClientID,
		PacketDestinationClientID: route.DestinationClientID,
		PacketTimeoutTimestamp:    time.Now().Add(time.Hour),
	}, slog.Default())

	invalid := NewTransfer(store.Packet{
		SourceChainID:             route.SourceChainID,
		DestinationChainID:        route.DestinationChainID,
		SourceTxHash:              "not-hex",
		PacketSequenceNumber:      2,
		PacketSourceClientID:      route.SourceClientID,
		PacketDestinationClientID: route.DestinationClientID,
		PacketTimeoutTimestamp:    time.Now().Add(time.Hour),
	}, slog.Default())

	db, err := store.NewSqliteInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.MigrateUp()
	require.NoError(t, err)

	destinationChainClient := mocks.NewMockClient(t)
	destinationChainClient.EXPECT().WaitForChain(mock.Anything).Return(nil).Once()
	destinationChainClient.EXPECT().ChainID().Return(route.DestinationChainID).Once()

	sourceChainClient := mocks.NewMockClient(t)

	var capturedTxIDs [][]byte
	sourceChainClient.EXPECT().TxPacketEvents(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, txHash []byte) ([]v2.PacketEvent, error) {
			capturedTxIDs = append(capturedTxIDs, txHash)

			return []v2.PacketEvent{
				{
					Height: 100,
					Kind:   v2.KindSendPacket,
					Packet: channeltypesv2.Packet{Sequence: 1, SourceClient: route.SourceClientID},
				},
			}, nil
		}).Once()

	mockProver := mocks.NewMockProver(t)
	mockProver.EXPECT().LatestProvableHeight(mock.Anything).Return(uint64(100), time.Time{}, nil)
	mockProver.EXPECT().ClientUpdatePayloads(mock.Anything, uint64(100)).Return([][]byte{{0x01}}, nil)
	mockProver.EXPECT().
		PacketProofs(mock.Anything, uint64(100), v2.ProofKindPacketCommitment, mock.Anything, []channeltypesv2.Acknowledgement(nil)).
		Return([][]byte{{0x02}}, nil)

	txBuilder := mocks.NewMockTxBuilder(t)
	txBuilder.EXPECT().BuildRelayTxs(mock.Anything, mock.Anything).
		Return([]v2.RelayTx{{To: common.HexToAddress("0xrouter").Bytes(), Data: []byte{0x01}}}, nil)

	txSubmitter := mocks.NewMockTxSubmitter(t)
	txSubmitter.EXPECT().SimulateRelayTx(mock.Anything,
		v2.RelayTx{To: common.HexToAddress("0xrouter").Bytes(), Data: []byte{0x01}}).Return(v2.RelaySimulation{
		Operations: []v2.OperationResult{
			{Type: v2.OperationTypeUpdate, Success: true},
			{Type: v2.OperationTypePacket, Success: true},
		},
	}, nil).Once()
	txSubmitter.EXPECT().Submit(mock.Anything, mock.Anything).Return(&v2.Submission{
		TxHash:         "0xrecv",
		SubmittedAt:    time.Now().UTC(),
		RelayerAddress: "0xrelayer",
	}, nil).Once()

	p, err := NewBatchRecvPacket(
		staticChains{route.SourceChainID: sourceChainClient, route.DestinationChainID: destinationChainClient},
		staticProvers{prover.Key(route.DestinationChainID, route.DestinationClientID): mockProver},
		staticTxBuilders{route.DestinationChainID: txBuilder},
		db,
		txSubmitter,
		route,
		slog.Default(),
	)
	require.NoError(t, err)

	_, err = p.Process(context.Background(), []*Transfer{valid, invalid})
	require.NoError(t, err)

	require.Len(t, capturedTxIDs, 1, "only the valid tx hash should be included")

	require.Error(t, invalid.ProcessingError)
	require.NoError(t, valid.ProcessingError)
	require.NotNil(t, valid.RecvTxHash)
}

// TestBatchRecvPacketToleratesPartialEventFetchFailure guards against a
// regression where one transfer's tx failing to yield packet events (a
// transient RPC error, say) would fail the whole batch and block every other
// transfer in it from being relayed, even though their own tx hashes fetched
// fine.
func TestBatchRecvPacketToleratesPartialEventFetchFailure(t *testing.T) {
	route := Route{
		SourceChainID:       "1",
		SourceClientID:      "base-0",
		DestinationChainID:  "8453",
		DestinationClientID: "ethereum-0",
	}

	healthyHash := "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	failingHash := "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	healthy := NewTransfer(store.Packet{
		SourceChainID:             route.SourceChainID,
		DestinationChainID:        route.DestinationChainID,
		SourceTxHash:              healthyHash,
		PacketSequenceNumber:      1,
		PacketSourceClientID:      route.SourceClientID,
		PacketDestinationClientID: route.DestinationClientID,
		PacketTimeoutTimestamp:    time.Now().Add(time.Hour),
	}, slog.Default())

	failing := NewTransfer(store.Packet{
		SourceChainID:             route.SourceChainID,
		DestinationChainID:        route.DestinationChainID,
		SourceTxHash:              failingHash,
		PacketSequenceNumber:      2,
		PacketSourceClientID:      route.SourceClientID,
		PacketDestinationClientID: route.DestinationClientID,
		PacketTimeoutTimestamp:    time.Now().Add(time.Hour),
	}, slog.Default())

	db, err := store.NewSqliteInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.MigrateUp()
	require.NoError(t, err)

	destinationChainClient := mocks.NewMockClient(t)
	destinationChainClient.EXPECT().WaitForChain(mock.Anything).Return(nil).Once()
	destinationChainClient.EXPECT().ChainID().Return(route.DestinationChainID).Once()

	healthyTxID, err := hex.DecodeString(healthyHash[2:])
	require.NoError(t, err)
	failingTxID, err := hex.DecodeString(failingHash[2:])
	require.NoError(t, err)

	sourceChainClient := mocks.NewMockClient(t)
	sourceChainClient.EXPECT().TxPacketEvents(mock.Anything, healthyTxID).Return([]v2.PacketEvent{
		{
			Height: 100,
			Kind:   v2.KindSendPacket,
			Packet: channeltypesv2.Packet{Sequence: 1, SourceClient: route.SourceClientID},
		},
	}, nil).Once()
	sourceChainClient.EXPECT().TxPacketEvents(mock.Anything, failingTxID).Return(nil, assert.AnError).Once()

	mockProver := mocks.NewMockProver(t)
	mockProver.EXPECT().LatestProvableHeight(mock.Anything).Return(uint64(100), time.Time{}, nil)
	mockProver.EXPECT().ClientUpdatePayloads(mock.Anything, uint64(100)).Return([][]byte{{0x01}}, nil)
	mockProver.EXPECT().
		PacketProofs(mock.Anything, uint64(100), v2.ProofKindPacketCommitment, mock.Anything, []channeltypesv2.Acknowledgement(nil)).
		Return([][]byte{{0x02}}, nil)

	txBuilder := mocks.NewMockTxBuilder(t)
	txBuilder.EXPECT().BuildRelayTxs(mock.Anything, mock.Anything).
		Return([]v2.RelayTx{{To: common.HexToAddress("0xrouter").Bytes(), Data: []byte{0x01}}}, nil)

	txSubmitter := mocks.NewMockTxSubmitter(t)
	txSubmitter.EXPECT().SimulateRelayTx(mock.Anything,
		v2.RelayTx{To: common.HexToAddress("0xrouter").Bytes(), Data: []byte{0x01}}).Return(v2.RelaySimulation{
		Operations: []v2.OperationResult{
			{Type: v2.OperationTypeUpdate, Success: true},
			{Type: v2.OperationTypePacket, Success: true},
		},
	}, nil).Once()
	txSubmitter.EXPECT().Submit(mock.Anything, mock.Anything).Return(&v2.Submission{
		TxHash:         "0xrecv",
		SubmittedAt:    time.Now().UTC(),
		RelayerAddress: "0xrelayer",
	}, nil).Once()

	p, err := NewBatchRecvPacket(
		staticChains{route.SourceChainID: sourceChainClient, route.DestinationChainID: destinationChainClient},
		staticProvers{prover.Key(route.DestinationChainID, route.DestinationClientID): mockProver},
		staticTxBuilders{route.DestinationChainID: txBuilder},
		db,
		txSubmitter,
		route,
		slog.Default(),
	)
	require.NoError(t, err)

	_, err = p.Process(context.Background(), []*Transfer{healthy, failing})
	require.NoError(t, err, "the batch as a whole must not fail just because one tx's events failed to fetch")

	require.Error(
		t,
		failing.ProcessingError,
		"the transfer whose tx failed to fetch must be excluded and retried later",
	)
	require.NoError(t, healthy.ProcessingError, "the transfer whose tx fetched fine must still be relayed")
	require.NotNil(t, healthy.RecvTxHash)
}

// oversizePacketTestSuite backs TestBatchRecvPacketFailsOversizePacket with a
// real in-memory store, so the test can observe the status the processor
// persists for a packet it refuses to relay.
type oversizePacketTestSuite struct {
	ctx   context.Context
	route Route
	db    *store.SqliteDB
}

func newOversizePacketTestSuite(t *testing.T) *oversizePacketTestSuite {
	t.Helper()

	db, err := store.NewSqliteInMemory()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	_, err = db.MigrateUp()
	require.NoError(t, err)

	return &oversizePacketTestSuite{
		ctx: context.Background(),
		route: Route{
			SourceChainID:       "1",
			SourceClientID:      "base-0",
			DestinationChainID:  "8453",
			DestinationClientID: "ethereum-0",
		},
		db: db,
	}
}

// newStoredTransfer persists a packet awaiting recv delivery and returns it
// as the processor would receive it from the dispatcher.
func (s *oversizePacketTestSuite) newStoredTransfer(t *testing.T, seq uint64, hash string) *Transfer {
	t.Helper()

	err := s.db.UpsertPacket(s.ctx, store.UpsertPacket{
		Status:                    store.RelayStatusPending,
		SourceChainID:             s.route.SourceChainID,
		DestinationChainID:        s.route.DestinationChainID,
		SourceTxHash:              hash,
		SourceTxTime:              time.Now(),
		PacketSequenceNumber:      seq,
		PacketSourceClientID:      s.route.SourceClientID,
		PacketDestinationClientID: s.route.DestinationClientID,
		PacketTimeoutTimestamp:    time.Now().Add(time.Hour),
	})
	require.NoError(t, err)

	key := store.PacketKey{SourceChainID: s.route.SourceChainID, SourceClientID: s.route.SourceClientID, Sequence: seq}
	require.NoError(t, s.db.UpdatePacketStatus(s.ctx, key, store.RelayStatusDeliverRecvPacket))

	return NewTransfer(s.storedPacket(t, hash), slog.Default())
}

func (s *oversizePacketTestSuite) storedPacket(t *testing.T, hash string) store.Packet {
	t.Helper()

	packets, err := s.db.ListPacketsBySourceTx(s.ctx, s.route.SourceChainID, hash)
	require.NoError(t, err)
	require.Len(t, packets, 1)

	return packets[0]
}

// sendPacketEvent returns a SendPacket event for the given tx hash whose
// packet carries a payload value of the given size.
func (s *oversizePacketTestSuite) sendPacketEvent(seq uint64, valueSize int) []v2.PacketEvent {
	packet := channeltypesv2.Packet{Sequence: seq, SourceClient: s.route.SourceClientID}
	if valueSize > 0 {
		packet.Payloads = []channeltypesv2.Payload{{Value: make([]byte, valueSize)}}
	}

	return []v2.PacketEvent{{Height: 100, Kind: v2.KindSendPacket, Packet: packet}}
}

func txID(t *testing.T, hash string) []byte {
	t.Helper()

	id, err := hex.DecodeString(hash[2:])
	require.NoError(t, err)

	return id
}

// TestBatchRecvPacketFailsOversizePacket guards against one packet over the
// attestor's per-packet cap poisoning every packet batched with it: the
// attestor refuses the whole request, PacketProofs is all-or-nothing, and
// with no terminal transition the same batch was rebuilt on every poll.
func TestBatchRecvPacketFailsOversizePacket(t *testing.T) {
	// ARRANGE
	suite := newOversizePacketTestSuite(t)
	route := suite.route

	healthyHash := "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	oversizeHash := "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	healthy := suite.newStoredTransfer(t, 1, healthyHash)
	oversize := suite.newStoredTransfer(t, 2, oversizeHash)

	sourceChainClient := mocks.NewMockClient(t)
	sourceChainClient.EXPECT().TxPacketEvents(mock.Anything, txID(t, healthyHash)).
		Return(suite.sendPacketEvent(1, 0), nil).Once()
	sourceChainClient.EXPECT().TxPacketEvents(mock.Anything, txID(t, oversizeHash)).
		Return(suite.sendPacketEvent(2, attestor.MaxPacketSizeBytes), nil).Once()

	destinationChainClient := mocks.NewMockClient(t)
	destinationChainClient.EXPECT().WaitForChain(mock.Anything).Return(nil).Once()
	destinationChainClient.EXPECT().ChainID().Return(route.DestinationChainID).Once()

	// the prover must only ever see the healthy packet
	mockProver := mocks.NewMockProver(t)
	mockProver.EXPECT().LatestProvableHeight(mock.Anything).Return(uint64(100), time.Time{}, nil)
	mockProver.EXPECT().ClientUpdatePayloads(mock.Anything, uint64(100)).Return([][]byte{{0x01}}, nil)
	mockProver.EXPECT().
		PacketProofs(
			mock.Anything, uint64(100), v2.ProofKindPacketCommitment,
			[]channeltypesv2.Packet{{Sequence: 1, SourceClient: route.SourceClientID}},
			[]channeltypesv2.Acknowledgement(nil),
		).
		Return([][]byte{{0x02}}, nil).Once()

	txBuilder := mocks.NewMockTxBuilder(t)
	txBuilder.EXPECT().BuildRelayTxs(mock.Anything, mock.Anything).
		Return([]v2.RelayTx{{To: common.HexToAddress("0xrouter").Bytes(), Data: []byte{0x01}}}, nil).Once()

	txSubmitter := mocks.NewMockTxSubmitter(t)
	txSubmitter.EXPECT().SimulateRelayTx(mock.Anything, mock.Anything).Return(v2.RelaySimulation{
		Operations: []v2.OperationResult{
			{Type: v2.OperationTypeUpdate, Success: true},
			{Type: v2.OperationTypePacket, Success: true},
		},
	}, nil).Once()
	txSubmitter.EXPECT().Submit(mock.Anything, mock.Anything).Return(&v2.Submission{
		TxHash:         "0xrecv",
		SubmittedAt:    time.Now().UTC(),
		RelayerAddress: "0xrelayer",
	}, nil).Once()

	p, err := NewBatchRecvPacket(
		staticChains{route.SourceChainID: sourceChainClient, route.DestinationChainID: destinationChainClient},
		staticProvers{prover.Key(route.DestinationChainID, route.DestinationClientID): mockProver},
		staticTxBuilders{route.DestinationChainID: txBuilder},
		suite.db,
		txSubmitter,
		route,
		slog.Default(),
	)
	require.NoError(t, err)

	// ACT
	_, err = p.Process(suite.ctx, []*Transfer{healthy, oversize})

	// ASSERT
	require.NoError(t, err, "the batch as a whole must not fail because of one oversize packet")

	require.NoError(t, healthy.ProcessingError, "the healthy packet must still be relayed")
	require.NotNil(t, healthy.RecvTxHash)
	require.Equal(t, store.RelayStatusDeliverRecvPacket, suite.storedPacket(t, healthyHash).Status)

	require.ErrorContains(t, oversize.ProcessingError, "packet size")
	require.Equal(t, store.RelayStatusFailed, oversize.Status)
	require.Equal(t, store.RelayStatusFailed, suite.storedPacket(t, oversizeHash).Status)
}

// TestBatchRecvPacketExcludesNotYetProvablePackets guards against a
// regression where one recently-observed packet in a batch -- more recent
// than the attestor quorum has currently caught up to -- would block every
// other packet in the batch from being relayed, instead of just that one
// packet waiting for the next run.
func TestBatchRecvPacketExcludesNotYetProvablePackets(t *testing.T) {
	route := Route{
		SourceChainID:       "1",
		SourceClientID:      "base-0",
		DestinationChainID:  "8453",
		DestinationClientID: "ethereum-0",
	}

	provableHash := "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	tooRecentHash := "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	provable := NewTransfer(store.Packet{
		SourceChainID:             route.SourceChainID,
		DestinationChainID:        route.DestinationChainID,
		SourceTxHash:              provableHash,
		PacketSequenceNumber:      1,
		PacketSourceClientID:      route.SourceClientID,
		PacketDestinationClientID: route.DestinationClientID,
		PacketTimeoutTimestamp:    time.Now().Add(time.Hour),
	}, slog.Default())

	tooRecent := NewTransfer(store.Packet{
		SourceChainID:             route.SourceChainID,
		DestinationChainID:        route.DestinationChainID,
		SourceTxHash:              tooRecentHash,
		PacketSequenceNumber:      2,
		PacketSourceClientID:      route.SourceClientID,
		PacketDestinationClientID: route.DestinationClientID,
		PacketTimeoutTimestamp:    time.Now().Add(time.Hour),
	}, slog.Default())

	db, err := store.NewSqliteInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.MigrateUp()
	require.NoError(t, err)

	destinationChainClient := mocks.NewMockClient(t)
	destinationChainClient.EXPECT().WaitForChain(mock.Anything).Return(nil).Once()
	destinationChainClient.EXPECT().ChainID().Return(route.DestinationChainID).Once()

	provableTxID, err := hex.DecodeString(provableHash[2:])
	require.NoError(t, err)
	tooRecentTxID, err := hex.DecodeString(tooRecentHash[2:])
	require.NoError(t, err)

	sourceChainClient := mocks.NewMockClient(t)
	sourceChainClient.EXPECT().TxPacketEvents(mock.Anything, provableTxID).Return([]v2.PacketEvent{
		{
			Height: 100,
			Kind:   v2.KindSendPacket,
			Packet: channeltypesv2.Packet{Sequence: 1, SourceClient: route.SourceClientID},
		},
	}, nil).Once()
	sourceChainClient.EXPECT().TxPacketEvents(mock.Anything, tooRecentTxID).Return([]v2.PacketEvent{
		{
			Height: 150,
			Kind:   v2.KindSendPacket,
			Packet: channeltypesv2.Packet{Sequence: 2, SourceClient: route.SourceClientID},
		},
	}, nil).Once()

	mockProver := mocks.NewMockProver(t)
	mockProver.EXPECT().LatestProvableHeight(mock.Anything).Return(uint64(100), time.Time{}, nil)
	mockProver.EXPECT().ClientUpdatePayloads(mock.Anything, uint64(100)).Return([][]byte{{0x01}}, nil)
	mockProver.EXPECT().
		PacketProofs(mock.Anything, uint64(100), v2.ProofKindPacketCommitment, mock.Anything, []channeltypesv2.Acknowledgement(nil)).
		Return([][]byte{{0x02}}, nil)

	txBuilder := mocks.NewMockTxBuilder(t)
	txBuilder.EXPECT().BuildRelayTxs(mock.Anything, mock.Anything).
		Return([]v2.RelayTx{{To: common.HexToAddress("0xrouter").Bytes(), Data: []byte{0x01}}}, nil)

	txSubmitter := mocks.NewMockTxSubmitter(t)
	txSubmitter.EXPECT().SimulateRelayTx(mock.Anything,
		v2.RelayTx{To: common.HexToAddress("0xrouter").Bytes(), Data: []byte{0x01}}).Return(v2.RelaySimulation{
		Operations: []v2.OperationResult{
			{Type: v2.OperationTypeUpdate, Success: true},
			{Type: v2.OperationTypePacket, Success: true},
		},
	}, nil).Once()
	txSubmitter.EXPECT().Submit(mock.Anything, mock.Anything).Return(&v2.Submission{
		TxHash:         "0xrecv",
		SubmittedAt:    time.Now().UTC(),
		RelayerAddress: "0xrelayer",
	}, nil).Once()

	p, err := NewBatchRecvPacket(
		staticChains{route.SourceChainID: sourceChainClient, route.DestinationChainID: destinationChainClient},
		staticProvers{prover.Key(route.DestinationChainID, route.DestinationClientID): mockProver},
		staticTxBuilders{route.DestinationChainID: txBuilder},
		db,
		txSubmitter,
		route,
		slog.Default(),
	)
	require.NoError(t, err)

	_, err = p.Process(context.Background(), []*Transfer{provable, tooRecent})
	require.NoError(t, err, "the batch as a whole must not fail just because one packet isn't provable yet")

	require.NoError(t, provable.ProcessingError)
	require.NotNil(t, provable.RecvTxHash)

	require.Error(
		t,
		tooRecent.ProcessingError,
		"the too-recent packet must be excluded and retried once attestors catch up",
	)
	require.Nil(t, tooRecent.RecvTxHash)
}

// TestBatchRecvPacketShouldProcessExcludesAlreadySettled guards against a
// regression where a transfer whose ack (or timeout) was recorded directly by
// CheckPacketCommitment -- because the source commitment was already gone --
// still had RecvTxHash nil and was not yet timed out, so BatchRecvPacket would
// try to deliver a recv for a packet already settled on the source chain.
func TestBatchRecvPacketShouldProcessExcludesAlreadySettled(t *testing.T) {
	p := BatchRecvPacket{}

	base := func() *Transfer {
		return NewTransfer(store.Packet{
			PacketTimeoutTimestamp: time.Now().Add(time.Hour),
		}, slog.Default())
	}

	t.Run("freshTransfer", func(t *testing.T) {
		require.True(t, p.ShouldProcess(base()))
	})

	t.Run("ackAlreadyRecordedWithoutRecv", func(t *testing.T) {
		tr := base()
		ackHash := "0xack"
		tr.AckTxHash = &ackHash

		require.False(t, p.ShouldProcess(tr))
	})

	t.Run("timeoutAlreadyRecordedWithoutRecv", func(t *testing.T) {
		tr := base()
		timeoutHash := "0xtimeout"
		tr.TimeoutTxHash = &timeoutHash

		require.False(t, p.ShouldProcess(tr))
	})
}
