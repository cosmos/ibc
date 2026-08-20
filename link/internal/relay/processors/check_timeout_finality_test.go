// SPDX-License-Identifier: Apache-2.0

package processors

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/relay/proofgen"
	"github.com/cosmos/ibc/link/internal/store"
	"github.com/cosmos/ibc/link/internal/tests/mocks"
)

func TestNewCheckTimeoutFinality(t *testing.T) {
	route := testSendFinalityRoute()

	t.Run("missingProofGeneratorErrors", func(t *testing.T) {
		_, err := NewCheckTimeoutFinality(staticProofGenerators{}, route)
		require.Error(t, err)
	})

	t.Run("resolvesProofGenerator", func(t *testing.T) {
		_, err := NewCheckTimeoutFinality(
			staticProofGenerators{
				proofgen.Key(route.SourceChainID, route.SourceClientID): mocks.NewMockProver(t),
			},
			route,
		)
		require.NoError(t, err)
	})
}

func TestCheckTimeoutFinalityProcess(t *testing.T) {
	route := testSendFinalityRoute()

	newTransfer := func(timeout time.Time) *Transfer {
		return NewTransfer(store.Packet{
			SourceChainID:             route.SourceChainID,
			DestinationChainID:        route.DestinationChainID,
			PacketSequenceNumber:      1,
			PacketSourceClientID:      route.SourceClientID,
			PacketDestinationClientID: route.DestinationClientID,
			PacketTimeoutTimestamp:    timeout,
		}, slog.Default())
	}

	t.Run("timestampPastTimeoutIsFinalized", func(t *testing.T) {
		proofGen := mocks.NewMockProver(t)
		proofGen.EXPECT().LatestProvableHeight(mock.Anything).Return(uint64(100), time.Unix(2000, 0), nil).Once()

		p, err := NewCheckTimeoutFinality(
			staticProofGenerators{proofgen.Key(route.SourceChainID, route.SourceClientID): proofGen},
			route,
		)
		require.NoError(t, err)

		tr := newTransfer(time.Unix(1000, 0))
		out, err := p.Process(context.Background(), tr)
		require.NoError(t, err)
		require.Same(t, tr, out)
	})

	t.Run("timestampBeforeTimeoutErrorsRetryable", func(t *testing.T) {
		proofGen := mocks.NewMockProver(t)
		proofGen.EXPECT().LatestProvableHeight(mock.Anything).Return(uint64(100), time.Unix(500, 0), nil).Once()

		p, err := NewCheckTimeoutFinality(
			staticProofGenerators{proofgen.Key(route.SourceChainID, route.SourceClientID): proofGen},
			route,
		)
		require.NoError(t, err)

		tr := newTransfer(time.Unix(1000, 0))
		_, err = p.Process(context.Background(), tr)
		require.ErrorIs(t, err, ErrTimeoutNotFinalized)
	})
}

func TestCheckTimeoutFinalityShouldProcess(t *testing.T) {
	p := CheckTimeoutFinality{}

	base := func() *Transfer {
		return NewTransfer(store.Packet{PacketTimeoutTimestamp: time.Now().Add(-time.Hour)}, slog.Default())
	}

	t.Run("timedOutAndUnsettled", func(t *testing.T) {
		require.True(t, p.ShouldProcess(base()))
	})

	t.Run("notYetTimedOut", func(t *testing.T) {
		tr := NewTransfer(store.Packet{PacketTimeoutTimestamp: time.Now().Add(time.Hour)}, slog.Default())
		require.False(t, p.ShouldProcess(tr))
	})

	t.Run("alreadyReceived", func(t *testing.T) {
		tr := base()
		recvHash := "0xrecv"
		tr.RecvTxHash = &recvHash
		require.False(t, p.ShouldProcess(tr))
	})

	t.Run("alreadyTimedOut", func(t *testing.T) {
		tr := base()
		timeoutHash := "0xtimeout"
		tr.TimeoutTxHash = &timeoutHash
		require.False(t, p.ShouldProcess(tr))
	})
}
