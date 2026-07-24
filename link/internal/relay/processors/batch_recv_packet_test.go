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
	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/relay/proofgen"
	"github.com/cosmos/ibc/link/internal/relay/txbuilder"
	"github.com/cosmos/ibc/link/internal/store"
	"github.com/cosmos/ibc/link/internal/tests/mocks"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

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

	sourceChainClient := mocks.NewMockClient(t)

	var capturedTxIDs [][]byte
	sourceChainClient.EXPECT().TxPacketEvents(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, txHash []byte) ([]v2.PacketEvent, error) {
			capturedTxIDs = append(capturedTxIDs, txHash)

			return []v2.PacketEvent{
				{Height: 100, Kind: v2.KindSendPacket, Packet: channeltypesv2.Packet{Sequence: 1, SourceClient: route.SourceClientID}},
			}, nil
		}).Once()

	proofGen := mocks.NewMockProofGenerator(t)
	proofGen.EXPECT().LatestProvableHeight(mock.Anything).Return(uint64(100), time.Time{}, nil)
	proofGen.EXPECT().StateProof(mock.Anything, uint64(100)).Return([]byte{0x01}, nil)
	proofGen.EXPECT().PacketProofs(mock.Anything, uint64(100), v2.ProofKindPacketCommitment, mock.Anything).
		Return([][]byte{{0x02}}, nil)

	txBuilder := mocks.NewMockTxBuilder(t)
	txBuilder.EXPECT().BuildRelayTxs(mock.Anything, mock.Anything).
		Return([]v2.RelayTx{{To: common.HexToAddress("0xrouter").Bytes(), Data: []byte{0x01}}}, nil)

	txSubmitter := mocks.NewMockTxSubmitter(t)
	txSubmitter.EXPECT().Submit(mock.Anything, mock.Anything).Return(&v2.Submission{
		TxHash:         "0xrecv",
		SubmittedAt:    time.Now().UTC(),
		RelayerAddress: "0xrelayer",
	}, nil).Once()

	p, err := NewBatchRecvPacket(
		staticChains{route.SourceChainID: sourceChainClient, route.DestinationChainID: destinationChainClient},
		staticProofGenerators{proofgen.Key(route.DestinationChainID, route.DestinationClientID): proofGen},
		staticTxBuilders{route.DestinationChainID: txBuilder},
		db,
		txSubmitter,
		route,
	)
	require.NoError(t, err)

	_, err = p.Process(context.Background(), []*Transfer{valid, invalid})
	require.NoError(t, err)

	require.Len(t, capturedTxIDs, 1, "only the valid tx hash should be included")

	require.NotNil(t, invalid.ProcessingError)
	require.Nil(t, valid.ProcessingError)
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

	healthyTxID, err := hex.DecodeString(healthyHash[2:])
	require.NoError(t, err)
	failingTxID, err := hex.DecodeString(failingHash[2:])
	require.NoError(t, err)

	sourceChainClient := mocks.NewMockClient(t)
	sourceChainClient.EXPECT().TxPacketEvents(mock.Anything, healthyTxID).Return([]v2.PacketEvent{
		{Height: 100, Kind: v2.KindSendPacket, Packet: channeltypesv2.Packet{Sequence: 1, SourceClient: route.SourceClientID}},
	}, nil).Once()
	sourceChainClient.EXPECT().TxPacketEvents(mock.Anything, failingTxID).Return(nil, assert.AnError).Once()

	proofGen := mocks.NewMockProofGenerator(t)
	proofGen.EXPECT().LatestProvableHeight(mock.Anything).Return(uint64(100), time.Time{}, nil)
	proofGen.EXPECT().StateProof(mock.Anything, uint64(100)).Return([]byte{0x01}, nil)
	proofGen.EXPECT().PacketProofs(mock.Anything, uint64(100), v2.ProofKindPacketCommitment, mock.Anything).
		Return([][]byte{{0x02}}, nil)

	txBuilder := mocks.NewMockTxBuilder(t)
	txBuilder.EXPECT().BuildRelayTxs(mock.Anything, mock.Anything).
		Return([]v2.RelayTx{{To: common.HexToAddress("0xrouter").Bytes(), Data: []byte{0x01}}}, nil)

	txSubmitter := mocks.NewMockTxSubmitter(t)
	txSubmitter.EXPECT().Submit(mock.Anything, mock.Anything).Return(&v2.Submission{
		TxHash:         "0xrecv",
		SubmittedAt:    time.Now().UTC(),
		RelayerAddress: "0xrelayer",
	}, nil).Once()

	p, err := NewBatchRecvPacket(
		staticChains{route.SourceChainID: sourceChainClient, route.DestinationChainID: destinationChainClient},
		staticProofGenerators{proofgen.Key(route.DestinationChainID, route.DestinationClientID): proofGen},
		staticTxBuilders{route.DestinationChainID: txBuilder},
		db,
		txSubmitter,
		route,
	)
	require.NoError(t, err)

	_, err = p.Process(context.Background(), []*Transfer{healthy, failing})
	require.NoError(t, err, "the batch as a whole must not fail just because one tx's events failed to fetch")

	require.NotNil(t, failing.ProcessingError, "the transfer whose tx failed to fetch must be excluded and retried later")
	require.Nil(t, healthy.ProcessingError, "the transfer whose tx fetched fine must still be relayed")
	require.NotNil(t, healthy.RecvTxHash)
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

	provableTxID, err := hex.DecodeString(provableHash[2:])
	require.NoError(t, err)
	tooRecentTxID, err := hex.DecodeString(tooRecentHash[2:])
	require.NoError(t, err)

	sourceChainClient := mocks.NewMockClient(t)
	sourceChainClient.EXPECT().TxPacketEvents(mock.Anything, provableTxID).Return([]v2.PacketEvent{
		{Height: 100, Kind: v2.KindSendPacket, Packet: channeltypesv2.Packet{Sequence: 1, SourceClient: route.SourceClientID}},
	}, nil).Once()
	sourceChainClient.EXPECT().TxPacketEvents(mock.Anything, tooRecentTxID).Return([]v2.PacketEvent{
		{Height: 150, Kind: v2.KindSendPacket, Packet: channeltypesv2.Packet{Sequence: 2, SourceClient: route.SourceClientID}},
	}, nil).Once()

	proofGen := mocks.NewMockProofGenerator(t)
	proofGen.EXPECT().LatestProvableHeight(mock.Anything).Return(uint64(100), time.Time{}, nil)
	proofGen.EXPECT().StateProof(mock.Anything, uint64(100)).Return([]byte{0x01}, nil)
	proofGen.EXPECT().PacketProofs(mock.Anything, uint64(100), v2.ProofKindPacketCommitment, mock.Anything).
		Return([][]byte{{0x02}}, nil)

	txBuilder := mocks.NewMockTxBuilder(t)
	txBuilder.EXPECT().BuildRelayTxs(mock.Anything, mock.Anything).
		Return([]v2.RelayTx{{To: common.HexToAddress("0xrouter").Bytes(), Data: []byte{0x01}}}, nil)

	txSubmitter := mocks.NewMockTxSubmitter(t)
	txSubmitter.EXPECT().Submit(mock.Anything, mock.Anything).Return(&v2.Submission{
		TxHash:         "0xrecv",
		SubmittedAt:    time.Now().UTC(),
		RelayerAddress: "0xrelayer",
	}, nil).Once()

	p, err := NewBatchRecvPacket(
		staticChains{route.SourceChainID: sourceChainClient, route.DestinationChainID: destinationChainClient},
		staticProofGenerators{proofgen.Key(route.DestinationChainID, route.DestinationClientID): proofGen},
		staticTxBuilders{route.DestinationChainID: txBuilder},
		db,
		txSubmitter,
		route,
	)
	require.NoError(t, err)

	_, err = p.Process(context.Background(), []*Transfer{provable, tooRecent})
	require.NoError(t, err, "the batch as a whole must not fail just because one packet isn't provable yet")

	require.Nil(t, provable.ProcessingError)
	require.NotNil(t, provable.RecvTxHash)

	require.NotNil(t, tooRecent.ProcessingError, "the too-recent packet must be excluded and retried once attestors catch up")
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
