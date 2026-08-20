// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// testListPackets runs against both engines: the filters are generated per
// engine from one .sql file, and only a real query proves they agree.
func testListPackets(t *testing.T, s Store) {
	t.Helper()

	ctx := context.Background()
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)

	const (
		chainOne = "list-1"
		chainTwo = "list-2"
	)

	// Packets insert as PENDING and transition, as the pipeline does.
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

	// UpsertPacket only accepts NOT_SELECTED or PENDING, so each packet is
	// inserted as PENDING and then transitioned to the status it seeds with.
	for _, packet := range seed {
		insert := packet
		insert.Status = RelayStatusPending

		require.NoError(t, s.CreateRelayRequest(ctx, insert.SourceChainID, insert.SourceTxHash))
		require.NoError(t, s.UpsertPacket(ctx, insert))
		require.NoError(t, s.UpdatePacketStatus(ctx, PacketKey{
			SourceChainID:  insert.SourceChainID,
			SourceClientID: insert.PacketSourceClientID,
			Sequence:       insert.PacketSequenceNumber,
		}, packet.Status))
	}

	all := AllRelayStatuses()

	hashesFor := func(t *testing.T, filter PacketFilter) []string {
		t.Helper()

		packets, err := s.ListPackets(ctx, filter, Page{Limit: 100})
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
			// Scoped by chain: testRepoReadWrite reuses low sequences.
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
			// chainOne holds no src-b packet.
			require.Empty(t, hashesFor(t, PacketFilter{
				Statuses:       all,
				SourceChainID:  str(chainOne),
				SourceClientID: str("src-b"),
			}))
		})

		t.Run("unknownValueIsEmptyNotError", func(t *testing.T) {
			require.Empty(t, hashesFor(t, PacketFilter{Statuses: all, SourceTxHash: str("0xmissing")}))
		})

		t.Run("limitIsAppliedAsGiven", func(t *testing.T) {
			// The store pages exactly as asked; defaults and has-more probing
			// belong to the service.
			filter := PacketFilter{Statuses: all, SourceChainID: str(chainOne)}

			page, err := s.ListPackets(ctx, filter, Page{Limit: 1})
			require.NoError(t, err)
			require.Len(t, page, 1)

			both, err := s.ListPackets(ctx, filter, Page{Limit: 5})
			require.NoError(t, err)
			require.Len(t, both, 2)
		})

		t.Run("noMatchesIsEmpty", func(t *testing.T) {
			packets, err := s.ListPackets(ctx,
				PacketFilter{Statuses: all, SourceTxHash: str("0xnope")}, Page{Limit: 10})
			require.NoError(t, err)
			require.Empty(t, packets)
		})

		t.Run("pagingCoversEveryRowExactlyOnce", func(t *testing.T) {
			filter := PacketFilter{Statuses: all, SourceChainID: str(chainOne)}

			first, err := s.ListPackets(ctx, filter, Page{Limit: 1})
			require.NoError(t, err)
			require.Len(t, first, 1)

			second, err := s.ListPackets(ctx, filter, Page{Limit: 1, Before: first[0].ID})
			require.NoError(t, err)
			require.Len(t, second, 1)

			require.NotEqual(t, first[0].SourceTxHash, second[0].SourceTxHash)
			require.Less(t, second[0].ID, first[0].ID)
		})

		t.Run("zeroCursorStartsAtTheNewestPacket", func(t *testing.T) {
			filter := PacketFilter{Statuses: all, SourceChainID: str(chainOne)}

			unbounded, err := s.ListPackets(ctx, filter, Page{Limit: 100})
			require.NoError(t, err)
			require.NotEmpty(t, unbounded)

			explicit, err := s.ListPackets(ctx, filter,
				Page{Limit: 100, Before: unbounded[0].ID + 1})
			require.NoError(t, err)
			require.Equal(t, unbounded, explicit)
		})

		// The point of paging by cursor rather than offset: a packet arriving
		// mid-walk must not push a row from page one onto page two, where an
		// offset pager would return it twice.
		t.Run("insertsDuringPagingDoNotShiftPages", func(t *testing.T) {
			filter := PacketFilter{Statuses: all, SourceChainID: str(chainOne)}

			first, err := s.ListPackets(ctx, filter, Page{Limit: 1})
			require.NoError(t, err)
			require.Len(t, first, 1)

			arrival := UpsertPacket{
				Status: RelayStatusPending, SourceChainID: chainOne, DestinationChainID: chainTwo,
				SourceTxHash: "0xlistarrival", SourceTxTime: base, PacketSequenceNumber: 99,
				PacketSourceClientID: "src-a", PacketDestinationClientID: "dst-a",
				PacketTimeoutTimestamp: base.Add(time.Hour),
			}
			require.NoError(t, s.CreateRelayRequest(ctx, arrival.SourceChainID, arrival.SourceTxHash))
			require.NoError(t, s.UpsertPacket(ctx, arrival))

			rest, err := s.ListPackets(ctx, filter, Page{Limit: 100, Before: first[0].ID})
			require.NoError(t, err)

			for _, packet := range rest {
				require.NotEqual(t, first[0].ID, packet.ID, "page one row reappeared on page two")
				require.NotEqual(t, "0xlistarrival", packet.SourceTxHash,
					"a packet newer than the cursor must not appear behind it")
			}
		})

		// Both engines must reject the same pages: sqlite would otherwise read
		// a negative limit as unbounded, and postgres narrows the limit to
		// int32.
		t.Run("invalidPagesAreRejected", func(t *testing.T) {
			for name, page := range map[string]Page{
				"zero limit":            {Limit: 0},
				"negative limit":        {Limit: -1},
				"limit overflows int32": {Limit: math.MaxInt32 + 1},
				"negative cursor":       {Limit: 10, Before: -1},
			} {
				t.Run(name, func(t *testing.T) {
					_, err := s.ListPackets(ctx, PacketFilter{Statuses: all}, page)
					require.Error(t, err)
				})
			}
		})

		t.Run("orderedMostRecentFirst", func(t *testing.T) {
			packets, err := s.ListPackets(ctx, PacketFilter{Statuses: all}, Page{Limit: 100})
			require.NoError(t, err)
			require.NotEmpty(t, packets)

			for i := 1; i < len(packets); i++ {
				require.Greater(t, packets[i-1].ID, packets[i].ID, "packets must be ordered id DESC")
			}
		})

		t.Run("emptyStatusesMatchesNothing", func(t *testing.T) {
			// Must not silently widen to everything.
			require.Empty(t, hashesFor(t, PacketFilter{Statuses: nil}))
		})
	})
}
