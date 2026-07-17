package processors

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/store"
	"github.com/cosmos/ibc/link/internal/txmgr"

	proto "github.com/cosmos/ibc/link/internal/types/proofapi"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

// TestBatchRecvPacketSequenceAlignment guards against a regression where a
// transfer whose tx hash fails to decode still contributed its sequence
// number to the proof api request, leaving the request's sequences
// out of sync with its tx ids.
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

	client := chains.NewMockClient(t)
	client.EXPECT().WaitForChain(mock.Anything).Return(nil).Once()

	proofAPI := proto.NewMockProofApiServiceClient(t)

	var captured *proto.RelayByTxRequest
	proofAPI.EXPECT().RelayByTx(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, req *connect.Request[proto.RelayByTxRequest]) (*connect.Response[proto.RelayByTxResponse], error) {
			captured = req.Msg

			return connect.NewResponse(&proto.RelayByTxResponse{Tx: []byte{0x01}, Address: "0xrouter"}), nil
		})

	txManager := txmgr.NewMockTxManager(t)
	txManager.EXPECT().Submit(mock.Anything, mock.Anything).Return(&v2.Submission{
		TxHash:         "0xrecv",
		SubmittedAt:    time.Now().UTC(),
		RelayerAddress: "0xrelayer",
	}, nil).Once()

	chainSet := staticChains{route.DestinationChainID: client}

	p := NewBatchRecvPacket(chainSet, db, proofAPI, txManager, route)

	_, err = p.Process(context.Background(), []*Transfer{valid, invalid})
	require.NoError(t, err)

	require.NotNil(t, captured)
	require.Len(t, captured.SourceTxIds, 1, "only the valid tx hash should be included")
	require.Len(t, captured.SrcPacketSequences, 1, "the decode-failed transfer's sequence must not be included")
	require.EqualValues(t, valid.PacketSequenceNumber, captured.SrcPacketSequences[0])

	require.NotNil(t, invalid.ProcessingError)
	require.Nil(t, valid.ProcessingError)
	require.NotNil(t, valid.RecvTxHash)
}

type staticChains map[string]chains.Client

func (s staticChains) Get(chainID string) (chains.Client, bool) {
	client, ok := s[chainID]
	return client, ok
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
