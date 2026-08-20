// SPDX-License-Identifier: Apache-2.0

package relayer

import (
	"context"
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
		for _, status := range dbStatusesForState(&state) {
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

// A packet mid-pipeline is not in the literal PENDING status, and must still be
// listed by a PENDING filter.
func TestDBStatusesForStatePendingCoversIntermediates(t *testing.T) {
	t.Parallel()

	pending := StatePending
	statuses := dbStatusesForState(&pending)

	for _, intermediate := range []store.RelayStatus{
		store.RelayStatusPending,
		store.RelayStatusAwaitingSendFinality,
		store.RelayStatusCheckRecvPacketDelivery,
		store.RelayStatusDeliverRecvPacket,
		store.RelayStatusWaitForWriteAck,
		store.RelayStatusAwaitingWriteAckFinality,
		store.RelayStatusDeliverAckPacket,
		store.RelayStatusAwaitingTimeoutFinality,
		store.RelayStatusDeliverTimeoutPacket,
	} {
		require.Containsf(t, statuses, intermediate,
			"a packet in %q is in flight and must match a PENDING filter", intermediate)
	}

	// Terminal statuses must not leak into PENDING.
	for _, terminal := range []store.RelayStatus{
		store.RelayStatusCompleteWithAck,
		store.RelayStatusCompleteWithTimeout,
		store.RelayStatusCompleteWithWriteAckError,
		store.RelayStatusFailed,
		store.RelayStatusNotSelected,
	} {
		require.NotContains(t, statuses, terminal)
	}
}

// An absent state filter widens to every status, since the query always applies
// the status set.
func TestDBStatusesForStateNilMeansEveryStatus(t *testing.T) {
	t.Parallel()

	require.ElementsMatch(t, store.AllRelayStatuses(), dbStatusesForState(nil))
}

// The inverse: an unmapped state narrows to nothing rather than widening.
func TestDBStatusesForStateUnspecifiedMatchesNothing(t *testing.T) {
	t.Parallel()

	unspecified := StateUnspecified
	require.Empty(t, dbStatusesForState(&unspecified))
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
	}{
		{"partial page", 5, 3, 3, false},
		{"exactly full", 3, 3, 3, false},
		{"probe returned", 3, 4, 3, true},
		{"empty", 5, 0, 0, false},
		{"default applied", 0, DefaultPacketPageLimit + 1, DefaultPacketPageLimit, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			st := NewMockStore(t)
			service := New(relayerConfig(), st, NewMockChainClients(t), nil)

			var asked int64

			st.EXPECT().ListPackets(ctx, mock.Anything, mock.Anything).
				Run(func(_ context.Context, _ store.PacketFilter, page store.Page) {
					asked = page.Limit
				}).
				Return(make([]store.Packet, tt.rowsFromDB), nil).Once()

			packets, hasMore, err := service.Packets(ctx, PacketFilter{}, store.Page{Limit: tt.limit})
			require.NoError(t, err)
			require.Len(t, packets, tt.wantPackets)
			require.Equal(t, tt.wantHasMore, hasMore)

			want := tt.limit
			if want <= 0 {
				want = DefaultPacketPageLimit
			}

			require.Equal(t, want+1, asked, "the store must be asked for one row past the page")
		})
	}
}

func TestPacketsCapsTheLimit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st := NewMockStore(t)
	service := New(relayerConfig(), st, NewMockChainClients(t), nil)

	var asked int64

	st.EXPECT().ListPackets(ctx, mock.Anything, mock.Anything).
		Run(func(_ context.Context, _ store.PacketFilter, page store.Page) {
			asked = page.Limit
		}).
		Return(nil, nil).Once()

	_, _, err := service.Packets(ctx, PacketFilter{}, store.Page{Limit: MaxPacketPageLimit + 500})
	require.NoError(t, err)
	require.Equal(t, int64(MaxPacketPageLimit+1), asked)
}
