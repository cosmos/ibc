// Package pipeline relays packets through their full lifecycle.
// Package transfer models the packet lifecycle the relayer drives.
package transfer

import (
	"log/slog"
	"time"

	"github.com/cosmos/ibc/link/internal/store"
)

// Transfer a packet moving through the relay pipeline. The embedded packet
// mirrors the store row; the remaining fields live in memory only.
type Transfer struct {
	store.Packet

	SourceTxFinalizedTime   *time.Time
	WriteAckTxFinalizedTime *time.Time

	// ProcessingError poisons the transfer for the rest of the run; errored
	// transfers pass through later stages untouched.
	ProcessingError error

	Logger *slog.Logger
}

func NewTransfer(packet store.Packet, logger *slog.Logger) *Transfer {
	return &Transfer{
		Packet: packet,
		Logger: logger.With(
			"sourceChainID", packet.SourceChainID,
			"sourceTxHash", packet.SourceTxHash,
			"sourceClientID", packet.PacketSourceClientID,
			"sequence", packet.PacketSequenceNumber,
		),
	}
}

func (t *Transfer) Error() string {
	// nil transfers appear in pipeline channels during shutdown
	if t == nil || t.ProcessingError == nil {
		return ""
	}

	return t.ProcessingError.Error()
}

func (t *Transfer) Key() store.PacketKey {
	return store.PacketKey{
		SourceChainID:  t.SourceChainID,
		SourceClientID: t.PacketSourceClientID,
		Sequence:       t.PacketSequenceNumber,
	}
}

func (t *Transfer) GetLogger() *slog.Logger {
	if t.Logger == nil {
		return slog.Default()
	}

	return t.Logger
}

func (t *Transfer) IsTimedOut() bool {
	return time.Now().After(t.PacketTimeoutTimestamp)
}

// IsComplete reports whether the transfer has finished its lifecycle: timed
// out, or received with its ack relayed back to the source chain.
func (t *Transfer) IsComplete() bool {
	if t.ProcessingError != nil {
		return false
	}

	if t.TimeoutTxHash != nil {
		return true
	}

	if t.RecvTxHash == nil || t.WriteAckTxHash == nil {
		return false
	}

	if t.AckTxHash != nil {
		return true
	}

	return true
}
