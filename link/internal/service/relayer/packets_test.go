// SPDX-License-Identifier: Apache-2.0

package relayer

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/store"
)

// Every relay status must be reachable by filtering on the state it maps to,
// or a packet in that status exists that no filter can list.
func TestDBStatusesForStateIsExhaustive(t *testing.T) {
	t.Parallel()

	covered := make(map[store.RelayStatus]PacketState)

	for _, state := range []PacketState{
		StateNotSelected,
		StatePending,
		StateSucceeded,
		StateTimedOut,
		StateRejected,
		StateRelayFailed,
	} {
		for _, status := range dbStatusesForState(state) {
			if previous, seen := covered[status]; seen {
				t.Fatalf("status %q claimed by both %v and %v", status, previous, state)
			}

			covered[status] = state
		}
	}

	for _, status := range store.AllRelayStatuses() {
		state, ok := covered[status]
		require.Truef(t, ok, "relay status %q is not reachable by any state filter", status)
		require.Equalf(t, mapPacketState(status), state,
			"status %q expands under %v but reports as %v", status, state, mapPacketState(status))
	}

	require.Len(t, covered, len(store.AllRelayStatuses()),
		"every relay status must be covered exactly once")
}

// The service pages by asking for one row past the page and trimming it, so a
// full page must not leak the probe row nor claim more when there is none.
func TestPacketsProbesForFurtherPages(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name        string
		limit       int64
		rowsFromDB  int
		wantPackets int
		wantHasMore bool
		// wantAsked is the limit the store must be given: one row past the
		// page, after the default and the cap are applied.
		wantAsked int64
	}{
		{"partial page", 5, 3, 3, false, 6},
		{"exactly full", 3, 3, 3, false, 4},
		{"probe returned", 3, 4, 3, true, 4},
		{"empty", 5, 0, 0, false, 6},
		{"default applied", 0, DefaultPacketPageLimit + 1, DefaultPacketPageLimit, true, DefaultPacketPageLimit + 1},
		{"limit capped", MaxPacketPageLimit + 500, 0, 0, false, MaxPacketPageLimit + 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			st := NewMockStore(t)
			service := New(relayerConfig(), st, NewMockChainClients(t), nil)

			var asked int64

			rows := make([]store.Packet, tt.rowsFromDB)
			for i := range rows {
				rows[i].ID = int64(tt.rowsFromDB - i)
			}

			st.EXPECT().ListPackets(ctx, mock.Anything, mock.Anything).
				Run(func(_ context.Context, _ store.PacketFilter, page store.Page) {
					asked = page.Limit
				}).
				Return(rows, nil).Once()

			page, err := service.Packets(ctx, PacketFilter{}, PacketQuery{Limit: tt.limit})
			require.NoError(t, err)
			require.Len(t, page.Packets, tt.wantPackets)
			require.Equal(t, tt.wantHasMore, page.HasMore)

			require.Equal(t, tt.wantAsked, asked, "the store must be asked for one row past the page")

			if !tt.wantHasMore {
				require.Empty(t, page.NextCursor, "a final page must not offer a cursor")
				return
			}

			// The cursor must name the last returned row, not the probe row,
			// or the next page would skip it.
			require.Equal(t, encodeCursor(rows[tt.wantPackets-1].ID), page.NextCursor)
		})
	}
}

// The cursor bounds the query rather than being interpreted by the caller, so
// the service must hand the store the id it names and reject anything else.
func TestPacketsCursorBoundsTheQuery(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st := NewMockStore(t)
	service := New(relayerConfig(), st, NewMockChainClients(t), nil)

	var asked int64

	st.EXPECT().ListPackets(ctx, mock.Anything, mock.Anything).
		Run(func(_ context.Context, _ store.PacketFilter, page store.Page) {
			asked = page.Before
		}).
		Return(nil, nil).Once()

	_, err := service.Packets(ctx, PacketFilter{}, PacketQuery{Cursor: encodeCursor(4096)})
	require.NoError(t, err)
	require.Equal(t, int64(4096), asked)
}

func TestPacketsRejectsMalformedCursors(t *testing.T) {
	t.Parallel()

	// "MA" decodes to "0", a position no packet can occupy.
	for _, cursor := range []string{"not-base64!", "abc", "MA"} {
		ctx := context.Background()
		service := New(relayerConfig(), NewMockStore(t), NewMockChainClients(t), nil)

		_, err := service.Packets(ctx, PacketFilter{}, PacketQuery{Cursor: cursor})
		require.ErrorIs(t, err, ErrInvalidInput, "cursor %q", cursor)
	}
}

// Chain ids name configuration, not data, so an unconfigured one is a caller
// error rather than a filter that matches nothing. Relay already rejects the
// same value; before this, Packets rejected it only when a tx hash was also
// supplied, since validation fell out of hash normalization.
func TestPacketsRejectsUnconfiguredChains(t *testing.T) {
	t.Parallel()

	known := chainIDEth
	unknown := "99999"
	hash := "0x" + strings.Repeat("ab", 32)

	for _, tt := range []struct {
		name   string
		filter PacketFilter
	}{
		{"source chain alone", PacketFilter{SourceChainID: unknown}},
		{"destination chain alone", PacketFilter{DestinationChainID: unknown}},
		{"source chain with tx hash", PacketFilter{SourceChainID: unknown, SourceTxHash: hash}},
		{"unknown destination with known source", PacketFilter{
			SourceChainID: known, DestinationChainID: unknown,
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			service := New(relayerConfig(), NewMockStore(t), NewMockChainClients(t), nil)

			_, err := service.Packets(context.Background(), tt.filter, PacketQuery{})
			require.ErrorIs(t, err, ErrInvalidInput)
			require.ErrorContains(t, err, unknown)
		})
	}
}

// Filters over data still match nothing rather than erroring: the relayer only
// knows packets it was asked to relay, so absence is not an error.
func TestPacketsUnknownDataFiltersReturnEmpty(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st := NewMockStore(t)
	service := New(relayerConfig(), st, NewMockChainClients(t), nil)

	st.EXPECT().ListPackets(ctx, mock.Anything, mock.Anything).Return(nil, nil).Once()

	hash := "0x" + strings.Repeat("ab", 32)
	sequence := uint64(999999)
	client := "no-such-client"

	page, err := service.Packets(ctx, PacketFilter{
		SourceTxHash: hash, SequenceNumber: sequence, SourceClientID: client,
	}, PacketQuery{})
	require.NoError(t, err)
	require.Empty(t, page.Packets)
}
