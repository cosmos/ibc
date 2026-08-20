// SPDX-License-Identifier: Apache-2.0

package processors

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/relay/prover"
	"github.com/cosmos/ibc/link/internal/store"
	"github.com/cosmos/ibc/link/internal/tests/mocks"
)

func TestNewCheckWriteAckFinality(t *testing.T) {
	route := testSendFinalityRoute()

	t.Run("missingChainClientErrors", func(t *testing.T) {
		_, err := NewCheckWriteAckFinality(
			staticChains{},
			staticProvers{
				prover.Key(route.SourceChainID, route.SourceClientID): mocks.NewMockProver(t),
			},
			route,
		)
		require.Error(t, err)
	})

	t.Run("missingProverErrors", func(t *testing.T) {
		_, err := NewCheckWriteAckFinality(
			staticChains{route.DestinationChainID: mocks.NewMockClient(t)},
			staticProvers{},
			route,
		)
		require.Error(t, err)
	})
}

func TestCheckWriteAckFinalityProcess(t *testing.T) {
	route := testSendFinalityRoute()

	newTransfer := func() *Transfer {
		writeAckHash := "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

		return NewTransfer(store.Packet{
			SourceChainID:             route.SourceChainID,
			DestinationChainID:        route.DestinationChainID,
			PacketSequenceNumber:      1,
			PacketSourceClientID:      route.SourceClientID,
			PacketDestinationClientID: route.DestinationClientID,
			WriteAckTxHash:            &writeAckHash,
		}, slog.Default())
	}

	t.Run("finalizedSetsFinalizedTime", func(t *testing.T) {
		destinationChainClient := mocks.NewMockClient(t)
		destinationChainClient.EXPECT().TxHeight(mock.Anything, mock.Anything).Return(uint64(100), nil).Once()

		proofGen := mocks.NewMockProver(t)
		proofGen.EXPECT().LatestProvableHeight(mock.Anything).Return(uint64(100), time.Time{}, nil).Once()

		p, err := NewCheckWriteAckFinality(
			staticChains{route.DestinationChainID: destinationChainClient},
			staticProvers{prover.Key(route.SourceChainID, route.SourceClientID): proofGen},
			route,
		)
		require.NoError(t, err)

		tr := newTransfer()
		out, err := p.Process(context.Background(), tr)
		require.NoError(t, err)
		require.NotNil(t, out.WriteAckTxFinalizedTime)
	})

	t.Run("notYetProvableErrorsRetryable", func(t *testing.T) {
		destinationChainClient := mocks.NewMockClient(t)
		destinationChainClient.EXPECT().TxHeight(mock.Anything, mock.Anything).Return(uint64(150), nil).Once()

		proofGen := mocks.NewMockProver(t)
		proofGen.EXPECT().LatestProvableHeight(mock.Anything).Return(uint64(100), time.Time{}, nil).Once()

		p, err := NewCheckWriteAckFinality(
			staticChains{route.DestinationChainID: destinationChainClient},
			staticProvers{prover.Key(route.SourceChainID, route.SourceClientID): proofGen},
			route,
		)
		require.NoError(t, err)

		tr := newTransfer()
		_, err = p.Process(context.Background(), tr)
		require.ErrorIs(t, err, ErrWriteAckNotFinalized)
	})
}

func TestCheckWriteAckFinalityShouldProcess(t *testing.T) {
	p := CheckWriteAckFinality{}

	base := func() *Transfer {
		writeAckHash := "0xack"
		return NewTransfer(store.Packet{WriteAckTxHash: &writeAckHash}, slog.Default())
	}

	t.Run("writeAckPendingSettlement", func(t *testing.T) {
		require.True(t, p.ShouldProcess(base()))
	})

	t.Run("noWriteAckYet", func(t *testing.T) {
		require.False(t, p.ShouldProcess(&Transfer{}))
	})

	t.Run("alreadyAcked", func(t *testing.T) {
		tr := base()
		ackHash := "0xack-tx"
		tr.AckTxHash = &ackHash
		require.False(t, p.ShouldProcess(tr))
	})

	t.Run("alreadyTimedOut", func(t *testing.T) {
		tr := base()
		timeoutHash := "0xtimeout"
		tr.TimeoutTxHash = &timeoutHash
		require.False(t, p.ShouldProcess(tr))
	})
}
