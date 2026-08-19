// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// testListPackets exercises every ListPackets filter against a seeded set. It
// runs against both engines from TestStore, because the filters are generated
// from one .sql file per engine and only a real query proves they agree.
func testListPackets(t *testing.T, s Store) {
	t.Helper()

	ctx := context.Background()
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)

	const (
		chainOne = "list-1"
		chainTwo = "list-2"
	)

	// Packets may only be inserted as NOT_SELECTED or PENDING, so the target
	// status is applied as a transition afterwards, exactly as the pipeline
	// does it.
	seed := []UpsertPacket{
		{
			Status: RelayStatusPending, SourceChainID: chainOne, DestinationChainID: chainTwo,
			SourceTxHash: "0xlist1", SourceTxTime: base, PacketSequenceNumber: 1,
			PacketSourceClientID: "src-a", PacketDestinationClientID: "dst-a",
			PacketTimeoutTimestamp: base.Add(time.Hour),
		},
		{
			Status: RelayStatusCompleteWithAck, SourceChainID: chainOne, DestinationChainID: chainTwo,
			SourceTxHash: "0xlist2", SourceTxTime: base, PacketSequenceNumber: 2,
			PacketSourceClientID: "src-a", PacketDestinationClientID: "dst-a",
			PacketTimeoutTimestamp: base.Add(time.Hour),
		},
		{
			// Different source client and chain, to prove filters discriminate.
			Status: RelayStatusAwaitingSendFinality, SourceChainID: chainTwo, DestinationChainID: chainOne,
			SourceTxHash: "0xlist3", SourceTxTime: base, PacketSequenceNumber: 3,
			PacketSourceClientID: "src-b", PacketDestinationClientID: "dst-b",
			PacketTimeoutTimestamp: base.Add(time.Hour),
		},
	}

	finalStatus := []RelayStatus{
		RelayStatusPending,
		RelayStatusCompleteWithAck,
		RelayStatusAwaitingSendFinality,
	}

	for i, packet := range seed {
		insert := packet
		insert.Status = RelayStatusPending

		require.NoError(t, s.CreateRelayRequest(ctx, insert.SourceChainID, insert.SourceTxHash))
		require.NoError(t, s.UpsertPacket(ctx, insert))
		require.NoError(t, s.UpdatePacketStatus(ctx, PacketKey{
			SourceChainID:  insert.SourceChainID,
			SourceClientID: insert.PacketSourceClientID,
			Sequence:       insert.PacketSequenceNumber,
		}, finalStatus[i]))
	}

	all := AllRelayStatuses()

	hashesFor := func(t *testing.T, filter PacketFilter) []string {
		t.Helper()

		packets, _, err := s.ListPackets(ctx, filter, Page{})
		require.NoError(t, err)

		hashes := make([]string, len(packets))
		for i, packet := range packets {
			hashes[i] = packet.SourceTxHash
		}

		return hashes
	}

	str := func(v string) *string { return &v }

	t.Run("listPackets", func(t *testing.T) {
		t.Run("bySourceChain", func(t *testing.T) {
			require.ElementsMatch(t, []string{"0xlist1", "0xlist2"},
				hashesFor(t, PacketFilter{Statuses: all, SourceChainID: str(chainOne)}))
		})

		t.Run("bySourceClient", func(t *testing.T) {
			require.ElementsMatch(t, []string{"0xlist3"},
				hashesFor(t, PacketFilter{Statuses: all, SourceClientID: str("src-b")}))
		})

		t.Run("byDestinationClient", func(t *testing.T) {
			require.ElementsMatch(t, []string{"0xlist1", "0xlist2"},
				hashesFor(t, PacketFilter{Statuses: all, DestinationClientID: str("dst-a")}))
		})

		t.Run("bySourceTxHash", func(t *testing.T) {
			require.Equal(t, []string{"0xlist2"},
				hashesFor(t, PacketFilter{Statuses: all, SourceTxHash: str("0xlist2")}))
		})

		t.Run("bySequence", func(t *testing.T) {
			// Scoped by chain: the shared database also holds packets seeded by
			// testRepoReadWrite, which reuse low sequence numbers.
			sequence := uint64(3)
			require.Equal(t, []string{"0xlist3"}, hashesFor(t, PacketFilter{
				Statuses: all, SourceChainID: str(chainTwo), SequenceNumber: &sequence,
			}))
		})

		t.Run("byStatusSubset", func(t *testing.T) {
			require.Equal(t, []string{"0xlist2"}, hashesFor(t, PacketFilter{
				Statuses:      []RelayStatus{RelayStatusCompleteWithAck},
				SourceChainID: str(chainOne),
			}))
		})

		t.Run("filtersCombineAsAnd", func(t *testing.T) {
			require.Empty(t, hashesFor(t, PacketFilter{
				Statuses:      all,
				SourceChainID: str(chainOne),
				// chainOne holds no src-b packet, so the AND yields nothing.
				SourceClientID: str("src-b"),
			}))
		})

		t.Run("unknownValueIsEmptyNotError", func(t *testing.T) {
			require.Empty(t, hashesFor(t, PacketFilter{Statuses: all, SourceTxHash: str("0xmissing")}))
		})

		t.Run("byCreatedRange", func(t *testing.T) {
			future := base.Add(100 * 365 * 24 * time.Hour)
			require.Empty(t, hashesFor(t, PacketFilter{Statuses: all, CreatedFrom: &future}))

			past := base.Add(-100 * 365 * 24 * time.Hour)
			require.NotEmpty(t, hashesFor(t, PacketFilter{Statuses: all, CreatedFrom: &past}))
			require.Empty(t, hashesFor(t, PacketFilter{Statuses: all, CreatedTo: &past}))
		})

		t.Run("hasMoreReportsFurtherPages", func(t *testing.T) {
			// chainOne holds two packets: a page of one has more, an exactly
			// full page does not, and the probe row is never handed back.
			filter := PacketFilter{Statuses: all, SourceChainID: str(chainOne)}

			first, hasMore, err := s.ListPackets(ctx, filter, Page{Limit: 1})
			require.NoError(t, err)
			require.Len(t, first, 1)
			require.True(t, hasMore)

			both, hasMore, err := s.ListPackets(ctx, filter, Page{Limit: 2})
			require.NoError(t, err)
			require.Len(t, both, 2)
			require.False(t, hasMore, "an exactly-full page must not claim more")

			last, hasMore, err := s.ListPackets(ctx, filter, Page{Limit: 1, Offset: 1})
			require.NoError(t, err)
			require.Len(t, last, 1)
			require.False(t, hasMore)
		})

		t.Run("noMatchesHasNoMore", func(t *testing.T) {
			packets, hasMore, err := s.ListPackets(ctx,
				PacketFilter{Statuses: all, SourceTxHash: str("0xnope")}, Page{})
			require.NoError(t, err)
			require.Empty(t, packets)
			require.False(t, hasMore)
		})

		t.Run("pagingCoversEveryRowExactlyOnce", func(t *testing.T) {
			filter := PacketFilter{Statuses: all, SourceChainID: str(chainOne)}

			first, _, err := s.ListPackets(ctx, filter, Page{Limit: 1, Offset: 0})
			require.NoError(t, err)
			second, _, err := s.ListPackets(ctx, filter, Page{Limit: 1, Offset: 1})
			require.NoError(t, err)

			require.Len(t, first, 1)
			require.Len(t, second, 1)
			require.NotEqual(t, first[0].SourceTxHash, second[0].SourceTxHash)
		})

		t.Run("orderedMostRecentFirst", func(t *testing.T) {
			packets, _, err := s.ListPackets(ctx, PacketFilter{Statuses: all}, Page{})
			require.NoError(t, err)
			require.NotEmpty(t, packets)

			for i := 1; i < len(packets); i++ {
				require.Greater(t, packets[i-1].ID, packets[i].ID, "packets must be ordered id DESC")
			}
		})

		t.Run("emptyStatusesMatchesNothing", func(t *testing.T) {
			// A state that maps to no relay status must not silently widen to
			// "everything"; it has to return an empty listing.
			require.Empty(t, hashesFor(t, PacketFilter{Statuses: nil}))
		})

		t.Run("limitIsCapped", func(t *testing.T) {
			packets, _, err := s.ListPackets(ctx, PacketFilter{Statuses: all}, Page{Limit: MaxPacketPageLimit + 500})
			require.NoError(t, err)
			require.LessOrEqual(t, len(packets), MaxPacketPageLimit)
		})
	})
}
