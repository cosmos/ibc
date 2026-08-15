// SPDX-License-Identifier: Apache-2.0

package processors

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/store"
)

type fakeStatusStorage struct {
	last store.RelayStatus
}

func (f *fakeStatusStorage) UpdatePacketStatus(_ context.Context, _ store.PacketKey, status store.RelayStatus) error {
	f.last = status

	return nil
}

func TestStateFinisherProcess(t *testing.T) {
	base := func() *Transfer {
		return NewTransfer(store.Packet{
			PacketTimeoutTimestamp: time.Now().Add(time.Hour),
		}, slog.Default())
	}

	t.Run("timeout", func(t *testing.T) {
		tr := base()
		hash := "0xtimeout"
		tr.TimeoutTxHash = &hash

		storage := &fakeStatusStorage{}
		_, err := NewStateFinisher(storage).Process(context.Background(), tr)
		require.NoError(t, err)

		assert.Equal(t, store.RelayStatusCompleteWithTimeout, tr.Status)
		assert.Equal(t, store.RelayStatusCompleteWithTimeout, storage.last)
	})

	t.Run("ackRelayedWithWriteAckSuccess", func(t *testing.T) {
		tr := base()
		recvHash, writeAckHash, ackHash := "0xrecv", "0xwriteack", "0xack"
		status := store.WriteAckStatusSuccess
		tr.RecvTxHash = &recvHash
		tr.WriteAckTxHash = &writeAckHash
		tr.WriteAckStatus = &status
		tr.AckTxHash = &ackHash

		storage := &fakeStatusStorage{}
		_, err := NewStateFinisher(storage).Process(context.Background(), tr)
		require.NoError(t, err)

		assert.Equal(t, store.RelayStatusCompleteWithAck, tr.Status)
	})

	t.Run("ackRelayedWithWriteAckError", func(t *testing.T) {
		tr := base()
		recvHash, writeAckHash, ackHash := "0xrecv", "0xwriteack", "0xack"
		status := store.WriteAckStatusError
		tr.RecvTxHash = &recvHash
		tr.WriteAckTxHash = &writeAckHash
		tr.WriteAckStatus = &status
		tr.AckTxHash = &ackHash

		storage := &fakeStatusStorage{}
		_, err := NewStateFinisher(storage).Process(context.Background(), tr)
		require.NoError(t, err)

		assert.Equal(t, store.RelayStatusCompleteWithWriteAckError, tr.Status)
		assert.Equal(t, store.RelayStatusCompleteWithWriteAckError, storage.last)
	})

	t.Run("ackRelayedWithUnknownWriteAckStatus", func(t *testing.T) {
		tr := base()
		recvHash, writeAckHash, ackHash := "0xrecv", "0xwriteack", "0xack"
		status := store.WriteAckStatusUnknown
		tr.RecvTxHash = &recvHash
		tr.WriteAckTxHash = &writeAckHash
		tr.WriteAckStatus = &status
		tr.AckTxHash = &ackHash

		storage := &fakeStatusStorage{}
		_, err := NewStateFinisher(storage).Process(context.Background(), tr)
		require.NoError(t, err)

		assert.Equal(t, store.RelayStatusCompleteWithAck, tr.Status)
	})

	t.Run("ackRelayedWithNoRecordedWriteAckStatus", func(t *testing.T) {
		tr := base()
		recvHash, writeAckHash, ackHash := "0xrecv", "0xwriteack", "0xack"
		tr.RecvTxHash = &recvHash
		tr.WriteAckTxHash = &writeAckHash
		tr.AckTxHash = &ackHash

		storage := &fakeStatusStorage{}
		_, err := NewStateFinisher(storage).Process(context.Background(), tr)
		require.NoError(t, err)

		assert.Equal(t, store.RelayStatusCompleteWithAck, tr.Status)
	})

	t.Run("notComplete", func(t *testing.T) {
		tr := base()

		storage := &fakeStatusStorage{}
		_, err := NewStateFinisher(storage).Process(context.Background(), tr)
		require.NoError(t, err)

		assert.Empty(t, tr.Status)
		assert.Empty(t, storage.last)
	})
}
