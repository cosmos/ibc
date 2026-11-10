// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testDispatchableStatuses checks the status list ListDispatchablePackets
// excludes in SQL against IsTerminal: a packet is dispatched again exactly
// when it can still move and was selected for relaying.
func testDispatchableStatuses(t *testing.T, s Store) {
	t.Helper()

	ctx := context.Background()
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	const chainID = "dispatchable-1"

	hashOf := make(map[RelayStatus]string)

	for i, status := range AllRelayStatuses() {
		packet := UpsertPacket{
			Status:                    RelayStatusPending,
			SourceChainID:             chainID,
			DestinationChainID:        "dispatchable-2",
			SourceTxHash:              fmt.Sprintf("0xdispatchable%d", i),
			SourceTxTime:              base,
			PacketSequenceNumber:      uint64(i + 1),
			PacketSourceClientID:      "dispatchable-src",
			PacketDestinationClientID: "dispatchable-dst",
			PacketTimeoutTimestamp:    base.Add(time.Hour),
		}
		if status == RelayStatusNotSelected {
			packet.Status = RelayStatusNotSelected
		}

		require.NoError(t, s.UpsertPacket(ctx, packet))
		require.NoError(t, s.UpdatePacketStatus(ctx, PacketKey{
			SourceChainID:  chainID,
			SourceClientID: packet.PacketSourceClientID,
			Sequence:       packet.PacketSequenceNumber,
		}, status))

		hashOf[status] = packet.SourceTxHash
	}

	dispatchable, err := s.ListDispatchablePackets(ctx)
	require.NoError(t, err)

	listed := make(map[string]bool)
	for _, packet := range dispatchable {
		listed[packet.SourceTxHash] = true
	}

	for status, hash := range hashOf {
		want := !status.IsTerminal() && status != RelayStatusNotSelected
		assert.Equalf(t, want, listed[hash], "%s dispatchable", status)
	}
}
