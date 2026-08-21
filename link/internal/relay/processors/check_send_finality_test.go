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

func testSendFinalityRoute() Route {
	return Route{
		SourceChainID:       "1",
		SourceClientID:      "base-0",
		DestinationChainID:  "8453",
		DestinationClientID: "ethereum-0",
	}
}

func TestNewCheckSendFinality(t *testing.T) {
	route := testSendFinalityRoute()

	t.Run("missingChainClientErrors", func(t *testing.T) {
		_, err := NewCheckSendFinality(
			staticChains{},
			staticProvers{
				prover.Key(route.DestinationChainID, route.DestinationClientID): mocks.NewMockProver(t),
			},
			route,
		)
		require.Error(t, err)
	})

	t.Run("missingProverErrors", func(t *testing.T) {
		_, err := NewCheckSendFinality(
			staticChains{route.SourceChainID: mocks.NewMockClient(t)},
			staticProvers{},
			route,
		)
		require.Error(t, err)
	})
}

func TestCheckSendFinalityProcess(t *testing.T) {
	route := testSendFinalityRoute()

	newTransfer := func() *Transfer {
		return NewTransfer(store.Packet{
			SourceChainID:             route.SourceChainID,
			DestinationChainID:        route.DestinationChainID,
			SourceTxHash:              "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			SourceTxTime:              time.Now(),
			PacketSequenceNumber:      1,
			PacketSourceClientID:      route.SourceClientID,
			PacketDestinationClientID: route.DestinationClientID,
		}, slog.Default())
	}

	t.Run("finalizedSetsFinalizedTime", func(t *testing.T) {
		sourceChainClient := mocks.NewMockClient(t)
		sourceChainClient.EXPECT().TxHeight(mock.Anything, mock.Anything).Return(uint64(100), nil).Once()

		mockProver := mocks.NewMockProver(t)
		mockProver.EXPECT().LatestProvableHeight(mock.Anything).Return(uint64(100), time.Time{}, nil).Once()

		p, err := NewCheckSendFinality(
			staticChains{route.SourceChainID: sourceChainClient},
			staticProvers{prover.Key(route.DestinationChainID, route.DestinationClientID): mockProver},
			route,
		)
		require.NoError(t, err)

		tr := newTransfer()
		out, err := p.Process(context.Background(), tr)
		require.NoError(t, err)
		require.NotNil(t, out.SourceTxFinalizedTime)
	})

	t.Run("notYetProvableErrorsRetryable", func(t *testing.T) {
		sourceChainClient := mocks.NewMockClient(t)
		sourceChainClient.EXPECT().TxHeight(mock.Anything, mock.Anything).Return(uint64(150), nil).Once()

		mockProver := mocks.NewMockProver(t)
		mockProver.EXPECT().LatestProvableHeight(mock.Anything).Return(uint64(100), time.Time{}, nil).Once()

		p, err := NewCheckSendFinality(
			staticChains{route.SourceChainID: sourceChainClient},
			staticProvers{prover.Key(route.DestinationChainID, route.DestinationClientID): mockProver},
			route,
		)
		require.NoError(t, err)

		tr := newTransfer()
		_, err = p.Process(context.Background(), tr)
		require.ErrorIs(t, err, ErrSendNotFinalized)
	})
}

func TestCheckSendFinalityShouldProcess(t *testing.T) {
	p := CheckSendFinality{}

	t.Run("noRecvYet", func(t *testing.T) {
		require.True(t, p.ShouldProcess(&Transfer{}))
	})

	t.Run("alreadyReceived", func(t *testing.T) {
		recvHash := "0xrecv"
		require.False(t, p.ShouldProcess(&Transfer{Packet: store.Packet{RecvTxHash: &recvHash}}))
	})
}
