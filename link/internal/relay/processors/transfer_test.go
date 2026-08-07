package processors

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/cosmos/ibc/link/internal/store"
)

func TestTransferIsComplete(t *testing.T) {
	base := func() *Transfer {
		return NewTransfer(store.Packet{
			PacketTimeoutTimestamp: time.Now().Add(time.Hour),
		}, slog.Default())
	}

	t.Run("neitherRecvNorTimeout", func(t *testing.T) {
		assert.False(t, base().IsComplete())
	})

	t.Run("timedOut", func(t *testing.T) {
		tr := base()
		hash := "0xtimeout"
		tr.TimeoutTxHash = &hash

		assert.True(t, tr.IsComplete())
	})

	t.Run("receivedWriteAckPendingAck", func(t *testing.T) {
		// received and write-acked, but the ack has not been relayed back to
		// the source chain yet: must not report complete.
		tr := base()
		recvHash := "0xrecv"
		writeAckHash := "0xwriteack"
		status := store.WriteAckStatusSuccess
		tr.RecvTxHash = &recvHash
		tr.WriteAckTxHash = &writeAckHash
		tr.WriteAckStatus = &status

		assert.False(t, tr.IsComplete())
	})

	t.Run("ackRelayed", func(t *testing.T) {
		tr := base()
		recvHash := "0xrecv"
		writeAckHash := "0xwriteack"
		ackHash := "0xack"
		status := store.WriteAckStatusSuccess
		tr.RecvTxHash = &recvHash
		tr.WriteAckTxHash = &writeAckHash
		tr.WriteAckStatus = &status
		tr.AckTxHash = &ackHash

		assert.True(t, tr.IsComplete())
	})

	t.Run("processingErrorOverridesEverything", func(t *testing.T) {
		tr := base()
		hash := "0xtimeout"
		tr.TimeoutTxHash = &hash
		tr.ProcessingError = assert.AnError

		assert.False(t, tr.IsComplete())
	})
}
