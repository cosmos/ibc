package pipeline

import (
	"context"
	"time"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/store"

	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

// WriteAckStorage persists write ack details and clears recv txs.
type WriteAckStorage interface {
	UpdatePacketWriteAck(ctx context.Context, key store.PacketKey, ack store.WriteAck) error
	ClearPacketRecvTx(ctx context.Context, key store.PacketKey) error
}

// WaitForWriteAck extracts the write acknowledgement from the recv tx. Only
// synchronous acks are supported: the write ack is expected in the recv tx.
type WaitForWriteAck struct {
	chains  ChainClients
	storage WriteAckStorage
}

func NewWaitForWriteAck(chainClients ChainClients, storage WriteAckStorage) WaitForWriteAck {
	return WaitForWriteAck{chains: chainClients, storage: storage}
}

func (p WaitForWriteAck) Process(ctx context.Context, transfer *Transfer) (*Transfer, error) {
	client, ok := p.chains.Get(transfer.DestinationChainID)
	if !ok {
		return nil, errors.Errorf("no configured chain client for destination chain %s", transfer.DestinationChainID)
	}

	if transfer.RecvTxHash == nil {
		return nil, errors.New("transfer has no recv tx hash, violates ShouldProcess")
	}

	recvTxHash := *transfer.RecvTxHash

	ackStatus, err := client.PacketWriteAckStatus(
		ctx,
		recvTxHash,
		transfer.PacketSequenceNumber,
		transfer.PacketSourceClientID,
		transfer.PacketDestinationClientID,
	)
	switch {
	case errors.Is(err, v2.ErrWriteAckNotFoundForPacket):
		// the recv tx exists but does not contain this packet's write ack.
		// this happens when a packet times out while its batch accumulates:
		// clear the recv tx and error so the packet is retried and then timed
		// out on the next run.
		if errClear := p.storage.ClearPacketRecvTx(ctx, transfer.Key()); errClear != nil {
			return nil, errors.Wrapf(errClear, "clearing recv tx %s", recvTxHash)
		}

		return nil, errors.Errorf("write ack for transfer not found in recv tx %s", recvTxHash)
	case errors.Is(err, v2.ErrWriteAckDecoding):
		// non-standard acknowledgement formats cannot be classified; record
		// the ack with an unknown status
		transfer.GetLogger().Warn("Could not decode write ack, status is unknown", "error", err)
		ackStatus = v2.WriteAckStatusUnknown
	case err != nil:
		return nil, errors.Wrapf(err, "finding write ack in recv tx %s", recvTxHash)
	}

	status := writeAckStatusFromV2(ackStatus)

	// the write ack shares the recv tx, so it shares its time
	var writeAckTime time.Time
	if transfer.RecvTxTime == nil {
		// a recv tx hash without a time is a bug, but not worth failing over
		transfer.GetLogger().Error("Transfer has a recv tx hash but no recv tx time", "recvTxHash", recvTxHash)
	} else {
		writeAckTime = *transfer.RecvTxTime
	}

	ack := store.WriteAck{TxHash: recvTxHash, TxTime: writeAckTime, Status: status}
	if err := p.storage.UpdatePacketWriteAck(ctx, transfer.Key(), ack); err != nil {
		return nil, errors.Wrapf(err, "recording write ack from tx %s", recvTxHash)
	}

	transfer.WriteAckTxHash = &recvTxHash
	transfer.WriteAckTxTime = &ack.TxTime
	transfer.WriteAckStatus = &status

	return transfer, nil
}

func (p WaitForWriteAck) Cancel(transfer *Transfer, err error) {
	if errors.Is(err, v2.ErrTxNotFound) {
		transfer.GetLogger().Debug("Write ack tx not yet found on chain")

		return
	}

	transfer.GetLogger().Error("Waiting for write ack", "error", err)
}

func (p WaitForWriteAck) ShouldProcess(transfer *Transfer) bool {
	return transfer.RecvTxHash != nil && transfer.WriteAckTxHash == nil && transfer.TimeoutTxHash == nil
}

func (p WaitForWriteAck) Status() store.RelayStatus {
	return store.RelayStatusWaitForWriteAck
}

func writeAckStatusFromV2(status v2.WriteAckStatus) store.WriteAckStatus {
	switch status {
	case v2.WriteAckStatusSuccess:
		return store.WriteAckStatusSuccess
	case v2.WriteAckStatusError:
		return store.WriteAckStatusError
	default:
		return store.WriteAckStatusUnknown
	}
}
