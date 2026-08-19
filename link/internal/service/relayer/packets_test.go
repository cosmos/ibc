// SPDX-License-Identifier: Apache-2.0

package relayer

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/store"
)

// TestDBStatusesForStateIsExhaustive is the guard for the state expansion. Every
// relay status must be reachable by filtering on the state it maps to,
// otherwise a packet in that status exists but can never be listed.
//
// It fails automatically when a relay status is added without AllRelayStatuses
// being updated, or when mapPacketState and the expansion disagree.
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

// TestDBStatusesForStatePendingCoversIntermediates pins the specific hazard: a
// packet mid-pipeline is not in the literal PENDING status, and must still be
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

// TestDBStatusesForStateNilMeansEveryStatus checks that an absent state filter
// widens to every status rather than matching nothing, since the query applies
// the status set unconditionally.
func TestDBStatusesForStateNilMeansEveryStatus(t *testing.T) {
	t.Parallel()

	require.ElementsMatch(t, store.AllRelayStatuses(), dbStatusesForState(nil))
}

// TestDBStatusesForStateUnspecifiedMatchesNothing checks the inverse: a state
// outside the mapped set must narrow to nothing rather than silently widening.
func TestDBStatusesForStateUnspecifiedMatchesNothing(t *testing.T) {
	t.Parallel()

	unspecified := StateUnspecified
	require.Empty(t, dbStatusesForState(&unspecified))
}
